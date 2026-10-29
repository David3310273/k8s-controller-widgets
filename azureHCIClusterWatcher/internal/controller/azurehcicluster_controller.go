/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	hciexamplecomv1 "my.demo/azure-hci-node-watcher/api/v1"
)

// AzureHCIClusterReconciler reconciles a AzureHCICluster object
type AzureHCIClusterReconciler struct {
	client.Client
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=hci.example.com,resources=azurehciclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hci.example.com,resources=azurehciclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=hci.example.com,resources=azurehciclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AzureHCICluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *AzureHCIClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// get existing CR (user's desired state)
	var cluster hciexamplecomv1.AzureHCICluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		log.Info("unable to fetch AzureHCICluster", "reason", err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// get current cluster state from Azure API
	result, err := r.getAzureHCICluster(ctx, cluster.Spec.ClusterName)
	if err != nil {
		log.Info("failed to get cluster info from Azure", "reason", err.Error())
		return ctrl.Result{}, err
	}

	// collect serial numbers from user's CR (what the user wants to keep)
	userNodeSNs := make(map[string]bool, len(cluster.Spec.Nodes))
	for _, node := range cluster.Spec.Nodes {
		userNodeSNs[node.SerialNumber] = true
	}

	currentNodeSNs := make(map[string]bool, len(result.Spec.Nodes))
	for _, node := range result.Spec.Nodes {
		currentNodeSNs[node.SerialNumber] = true
	}

	mu := &sync.Mutex{}
	var wg sync.WaitGroup
	var hasFailure bool

	// collect nodes to remove (exist in Azure but removed from user's CR)
	removals := make([]hciexamplecomv1.NodeInfo, 0)
	for _, azureNode := range result.Spec.Nodes {
		if !userNodeSNs[azureNode.SerialNumber] {
			removals = append(removals, azureNode)
		}
	}

	// collect nodes to add (exist in user's CR but not in Azure)
	pendingAdds := make([]int, 0)
	for idx, userNode := range cluster.Spec.Nodes {
		if !currentNodeSNs[userNode.SerialNumber] {
			cluster.Spec.Nodes[idx].State = "Pending"
			pendingAdds = append(pendingAdds, idx)
		}
	}

	needsUpdate := len(removals) > 0 || len(pendingAdds) > 0
	if needsUpdate {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "Progressing",
			Status:  metav1.ConditionTrue,
			Reason:  "Reconciling",
			Message: "Reconciling cluster nodes with Azure",
		})
		if err := r.Status().Update(ctx, &cluster); err != nil {
			log.Info("failed to update status to Progressing", "reason", err.Error())
			return ctrl.Result{}, err
		}
	}

	// fire goroutines to remove nodes
	for _, node := range removals {
		log.Info("node removed from CR, triggering external removal",
			"serialNumber", node.SerialNumber,
			"model", node.Model)
		wg.Add(1)
		go func(node hciexamplecomv1.NodeInfo) {
			defer wg.Done()
			nodeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- r.removeAzureHCINode(nodeCtx, node) }()
			select {
			case err := <-done:
				if err != nil {
					log.Info("failed to remove node from Azure", "reason", err.Error())
					mu.Lock()
					cluster.Spec.Nodes = append(cluster.Spec.Nodes, node)
					hasFailure = true
					mu.Unlock()
				}
			case <-nodeCtx.Done():
				log.Info("remove node timed out", "serialNumber", node.SerialNumber, "reason", nodeCtx.Err())
				mu.Lock()
				cluster.Spec.Nodes = append(cluster.Spec.Nodes, node)
				hasFailure = true
				mu.Unlock()
			}
		}(node)
	}

	// persist Pending state immediately so users see it right away
	if len(pendingAdds) > 0 {
		if err := r.Update(ctx, &cluster); err != nil {
			log.Info("failed to update AzureHCICluster with Pending state", "reason", err.Error())
			return ctrl.Result{}, err
		}
	}

	for _, idx := range pendingAdds {
		userNode := cluster.Spec.Nodes[idx]
		log.Info("node not in Azure, triggering external addition",
			"serialNumber", userNode.SerialNumber,
			"model", userNode.Model)
		wg.Add(1)
		go func(i int, node hciexamplecomv1.NodeInfo) {
			defer wg.Done()
			nodeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- r.addAzureHCINode(nodeCtx, node) }()
			select {
			case err := <-done:
				if err != nil {
					log.Info("failed to add node to Azure", "reason", err.Error())
					mu.Lock()
					cluster.Spec.Nodes[i].State = "Failed"
					hasFailure = true
					mu.Unlock()
				} else {
					cluster.Spec.Nodes[i].State = "Connected"
				}
			case <-nodeCtx.Done():
				log.Info("add node timed out", "serialNumber", node.SerialNumber, "reason", nodeCtx.Err())
				mu.Lock()
				cluster.Spec.Nodes[i].State = "Failed"
				hasFailure = true
				mu.Unlock()
			}
		}(idx, userNode)
	}

	wg.Wait()

	// set resource status
	if hasFailure {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionFalse,
			Reason:  "ReconcileFailed",
			Message: "One or more node operations failed or timed out",
		})
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "Degraded",
			Status:  metav1.ConditionTrue,
			Reason:  "ReconcileFailed",
			Message: "One or more node operations failed or timed out",
		})
	} else {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionTrue,
			Reason:  "ReconcileSucceeded",
			Message: "All node operations completed successfully",
		})
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:    "Degraded",
			Status:  metav1.ConditionFalse,
			Reason:  "ReconcileSucceeded",
			Message: "All node operations completed successfully",
		})
	}
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:    "Progressing",
		Status:  metav1.ConditionFalse,
		Reason:  "ReconcileComplete",
		Message: "Reconciliation completed",
	})

	if err := r.Update(ctx, &cluster); err != nil {
		log.Info("failed to update AzureHCICluster", "reason", err.Error())
		return ctrl.Result{}, err
	}

	log.Info("reconciled AzureHCICluster nodes",
		"cluster", cluster.Spec.ClusterName,
		"connected", cluster.Status.Connected)

	return ctrl.Result{}, nil
}

func (r *AzureHCIClusterReconciler) addAzureHCINode(ctx context.Context, node hciexamplecomv1.NodeInfo) error {
	// mock method: call Azure API to add this node
	// https://learn.microsoft.com/en-us/rest/api/stackhci/
	logf.FromContext(ctx).Info("adding Azure HCI node",
		"serialNumber", node.SerialNumber,
		"model", node.Model)

	return nil
}

func (r *AzureHCIClusterReconciler) removeAzureHCINode(ctx context.Context, node hciexamplecomv1.NodeInfo) error {
	// mock method: call Azure API to decommission / remove this node
	// https://learn.microsoft.com/en-us/rest/api/stackhci/
	logf.FromContext(ctx).Info("stopping Azure HCI node",
		"serialNumber", node.SerialNumber,
		"model", node.Model)

	return nil
}

func (r *AzureHCIClusterReconciler) getAzureHCICluster(ctx context.Context, clusterName string) (hciexamplecomv1.AzureHCICluster, error) {
	// mock data: should get info using azure ARM api with your subscription id, doc:
	// https://learn.microsoft.com/en-us/rest/api/stackhci/clusters/list-by-subscription?view=rest-stackhci-2024-04-01
	// https://learn.microsoft.com/en-us/rest/api/stackhci/arc-settings/list-by-cluster
	return hciexamplecomv1.AzureHCICluster{
		Status: hciexamplecomv1.AzureHCIClusterStatus{
			Connected: "Connected",
		},
		Spec: hciexamplecomv1.AzureHCIClusterSpec{
			ClusterName:        clusterName,
			ConnectivityStatus: "Connected",
			Nodes: []hciexamplecomv1.NodeInfo{
				{
					State:        "Connected",
					Manufacturer: "Dell",
					Model:        "PowerEdge R950",
					NetworkInterfaces: []hciexamplecomv1.NetworkInterface{
						{
							Name:       "eth0",
							MacAddress: "00:1A:36:00:00:01",
						},
					},
				},
			},
		},
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AzureHCIClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hciexamplecomv1.AzureHCICluster{}).
		Named("AzureHCICluster").
		Complete(r)
}
