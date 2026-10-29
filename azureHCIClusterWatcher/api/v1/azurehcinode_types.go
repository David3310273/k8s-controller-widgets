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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Subnet defines the subnet configuration for an IP address.
type Subnet struct {
	// addressPrefix is the subnet prefix in CIDR notation.
	AddressPrefix string `json:"addressPrefix"`
}

// IPAddress defines an IP address assigned to a network interface.
type IPAddress struct {
	// address is the IP address.
	Address string `json:"address"`

	// ipAddressVersion is the IP version (e.g. IPv4, IPv6).
	IPAddressVersion string `json:"ipAddressVersion"`

	// subnet is the subnet configuration for this IP address.
	// +optional
	Subnet *Subnet `json:"subnet,omitempty"`
}

// NetworkInterface defines a network interface on a node.
type NetworkInterface struct {
	// name is the name of the network interface.
	Name string `json:"name"`

	// macAddress is the MAC address of the network interface.
	MacAddress string `json:"macAddress"`

	// ipAddresses is the list of IP addresses assigned to this interface.
	// +optional
	IPAddresses []IPAddress `json:"ipAddresses,omitempty"`
}

// NodeInfo defines the information about a physical node in the Azure HCI cluster.
type NodeInfo struct {
	// manufacturer is the node manufacturer name.
	Manufacturer string `json:"manufacturer"`

	// model is the node model name.
	Model string `json:"model"`

	// osName is the operating system name.
	OsName string `json:"osName"`

	// osVersion is the operating system version.
	OsVersion string `json:"osVersion"`

	// serialNumber is the node serial number.
	SerialNumber string `json:"serialNumber"`

	// coreCount is the number of CPU cores.
	CoreCount int32 `json:"coreCount"`

	// memoryInGiB is the memory size in GiB.
	MemoryInGiB int32 `json:"memoryInGiB"`

	// oemActivation is the OEM activation status.
	OemActivation string `json:"oemActivation"`

	// state is the current state of the node.
	State string `json:"state"`

	// networkInterfaces is the list of network interfaces on this node.
	// +optional
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
}

// AzureHCIClusterSpec defines the desired state of AzureHCICluster
type AzureHCIClusterSpec struct {
	// nodes is the list of physical nodes in the cluster.
	// +optional
	Nodes []NodeInfo `json:"nodes,omitempty"`

	// clusterName is the name of the Azure HCI cluster.
	ClusterName string `json:"clusterName"`

	// connectivityStatus is the connectivity status of the cluster.
	ConnectivityStatus string `json:"connectivityStatus"`

	// clusterId is the unique identifier for the cluster.
	ClusterId string `json:"clusterId"`

	// cloudManagementEndpoint is the cloud management endpoint URL.
	CloudManagementEndpoint string `json:"cloudManagementEndpoint"`

	// manufacturer is the cluster manufacturer name.
	Manufacturer string `json:"manufacturer"`
}

// AzureHCIClusterStatus defines the observed state of AzureHCICluster.
type AzureHCIClusterStatus struct {
	// connected represents the observed connectivity status of the cluster.
	// +optional
	Connected string `json:"connected,omitempty"`

	// conditions represent the current state of the AzureHCICluster resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AzureHCICluster is the Schema for the azurehciclusters API
type AzureHCICluster struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AzureHCICluster
	// +required
	Spec AzureHCIClusterSpec `json:"spec"`

	// status defines the observed state of AzureHCICluster
	// +optional
	Status AzureHCIClusterStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AzureHCIClusterList contains a list of AzureHCICluster
type AzureHCIClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AzureHCICluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &AzureHCICluster{}, &AzureHCIClusterList{})
		return nil
	})
}
