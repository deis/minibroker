/*
Copyright 2019 The Kubernetes Authors.

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

package broker

import (
	"errors"
	"sync"

	"github.com/kubernetes-sigs/minibroker/pkg/minibroker"
	osb "github.com/pmorie/go-open-service-broker-client/v2"
	"github.com/pmorie/osb-broker-lib/pkg/broker"
	klog "k8s.io/klog/v2"
)

// NewBroker is a hook that is called with the Options the program is run
// with. NewBroker is the place where you will initialize your
// Broker the parameters passed in.
func NewBroker(o Options) (*Broker, error) {
	klog.V(5).Infof("broker: creating a new broker with options %+v", o)
	mb := minibroker.NewClient(o.HelmRepoURL, o.ServiceCatalogEnabledOnly)
	err := mb.Init()
	if err != nil {
		return nil, err
	}

	// For example, if your Broker requires a parameter from the command
	// line, you would unpack it from the Options and set it on the
	// Broker here.
	return &Broker{
		Client:           mb,
		async:            true,
		defaultNamespace: o.DefaultNamespace,
	}, nil
}

// Broker provides an implementation of broker.Interface
type Broker struct {
	Client *minibroker.Client

	// Indiciates if the broker should handle the requests asynchronously.
	async bool
	// Synchronize go routines.
	sync.RWMutex
	// Default namespace to run brokers if not specified during request
	defaultNamespace string
}

var _ broker.Interface = &Broker{}

func (b *Broker) GetCatalog(c *broker.RequestContext) (*broker.CatalogResponse, error) {
	klog.V(4).Infoln("broker: getting catalog")
	services, err := b.Client.ListServices()
	if err != nil {
		return nil, err
	}

	response := &broker.CatalogResponse{
		CatalogResponse: osb.CatalogResponse{
			Services: services,
		},
	}

	klog.V(4).Infoln("broker: got catalog")
	return response, nil
}

func (b *Broker) Provision(request *osb.ProvisionRequest, _ *broker.RequestContext) (*broker.ProvisionResponse, error) {
	b.Lock()
	defer b.Unlock()

	namespace := b.defaultNamespace
	if request.Context["namespace"] != nil {
		namespace = request.Context["namespace"].(string)
	}

	if namespace == "" {
		klog.V(4).Infof("broker: failed to provision %q with empty namespace", request.InstanceID)
		return nil, errors.New("Cannot provision with empty namespace")
	}

	klog.V(4).Infof("broker: provisioning request %+v in namespace %q", request, namespace)

	operationName, err := b.Client.Provision(request.InstanceID, request.ServiceID, request.PlanID, namespace, request.AcceptsIncomplete, request.Parameters)
	if err != nil {
		klog.V(4).Infof("broker: failed to provision request %q: %v", request.InstanceID, err)
		return nil, err
	}

	response := broker.ProvisionResponse{}
	if request.AcceptsIncomplete {
		response.Async = true
		operationKey := osb.OperationKey(operationName)
		response.OperationKey = &operationKey
	}

	klog.V(4).Infof("broker: provisioned %q in namespace %q", request.InstanceID, namespace)
	return &response, nil
}

func (b *Broker) Deprovision(request *osb.DeprovisionRequest, _ *broker.RequestContext) (*broker.DeprovisionResponse, error) {
	klog.V(4).Infof("broker: deprovisioning request %+v", request)

	b.Lock()
	defer b.Unlock()

	operationName, err := b.Client.Deprovision(request.InstanceID, request.AcceptsIncomplete)
	if err != nil {
		klog.V(4).Infof("broker: failed to deprovision %q: %v", request.InstanceID, err)
		return nil, err
	}

	response := broker.DeprovisionResponse{}
	if request.AcceptsIncomplete {
		response.Async = b.async
		operationKey := osb.OperationKey(operationName)
		response.OperationKey = &operationKey
	}

	klog.V(4).Infof("broker: deprovisioned %q", request.InstanceID)
	return &response, nil
}

// LastOperation provides information on the state of the last asynchronous operation
func (b *Broker) LastOperation(request *osb.LastOperationRequest, _ *broker.RequestContext) (*broker.LastOperationResponse, error) {
	klog.V(4).Infof("broker: getting last operation request %+v", request)

	b.Lock()
	defer b.Unlock()

	response, err := b.Client.LastOperationState(request.InstanceID, request.OperationKey)
	if err != nil {
		klog.V(4).Infof("broker: failed to get last operation for instance %q: %v", request.InstanceID, err)
		return nil, err
	}

	wrappedResponse := broker.LastOperationResponse{LastOperationResponse: *response}

	klog.V(4).Infof("broker: got last operation for %q: %+v", request.InstanceID, response)
	return &wrappedResponse, nil
}

func (b *Broker) Bind(request *osb.BindRequest, _ *broker.RequestContext) (*broker.BindResponse, error) {
	klog.V(4).Infof("broker: binding request %+v", request)

	b.Lock()
	defer b.Unlock()

	operationName, err := b.Client.Bind(request.InstanceID, request.ServiceID, request.BindingID, request.AcceptsIncomplete, request.Parameters)
	if err != nil {
		klog.V(4).Infof("broker: failed to bind %q: %v", request.InstanceID, err)
		return nil, err
	}

	operationKey := osb.OperationKey(operationName)
	if request.AcceptsIncomplete {
		// If we accept incomplete, we can just return directly
		response := broker.BindResponse{
			BindResponse: osb.BindResponse{
				Async:        true,
				OperationKey: &operationKey,
			},
		}
		return &response, nil
	}

	// Get the response back out of the configmaps
	operationState, err := b.Client.LastBindingOperationState(request.InstanceID, request.BindingID)
	if err != nil {
		klog.V(4).Infof("broker: failed to bind %q: %v", request.InstanceID, err)
		return nil, err
	}
	if operationState.State != osb.StateSucceeded {
		klog.V(4).Infof("broker: failed to bind instance %q: state is %q", request.InstanceID, operationState.State)
		return nil, errors.New("Failed to bind instance")
	}
	binding, err := b.Client.GetBinding(request.InstanceID, request.BindingID)
	if err != nil {
		klog.V(4).Infof("broker: failed to bind %q: %v", request.InstanceID, err)
		return nil, err
	}

	bindResponse := broker.BindResponse{
		BindResponse: osb.BindResponse{
			Credentials:     binding.Credentials,
			SyslogDrainURL:  binding.SyslogDrainURL,
			RouteServiceURL: binding.RouteServiceURL,
			VolumeMounts:    binding.VolumeMounts,
		},
	}

	klog.V(4).Infof("broker: bound %q", request.InstanceID)

	return &bindResponse, nil
}

func (b *Broker) GetBinding(request *osb.GetBindingRequest, _ *broker.RequestContext) (*broker.GetBindingResponse, error) {
	klog.V(4).Infof("broker: getting binding request %+v", request)

	binding, err := b.Client.GetBinding(request.InstanceID, request.BindingID)
	if err != nil {
		klog.V(4).Infof("broker: failed to get binding %q for instance %q: %v", request.BindingID, request.InstanceID, err)
		return nil, err
	}
	response := broker.GetBindingResponse{
		GetBindingResponse: *binding,
	}

	klog.V(4).Infof("broker: got binding %q", request.BindingID)

	return &response, nil
}

func (b *Broker) BindingLastOperation(request *osb.BindingLastOperationRequest, _ *broker.RequestContext) (*broker.LastOperationResponse, error) {
	klog.V(4).Infof("broker: getting binding last operation request %+v", request)

	state, err := b.Client.LastBindingOperationState(request.InstanceID, request.BindingID)
	if err != nil {
		klog.V(4).Infof("broker: failed to get binding %q last operation for instance %q: %v", request.BindingID, request.InstanceID, err)
		return nil, err
	}

	response := broker.LastOperationResponse{LastOperationResponse: *state}

	klog.V(4).Infof("broker: got last binding operation for %q: %+v", request.InstanceID, *state)

	return &response, nil
}

func (b *Broker) Unbind(request *osb.UnbindRequest, c *broker.RequestContext) (*broker.UnbindResponse, error) {
	klog.V(4).Infof("broker: unbinding request %+v", request)

	if err := b.Client.Unbind(request.InstanceID, request.BindingID); err != nil {
		klog.V(4).Infof("broker: failed to unbind instance %q: %v", request.InstanceID, err)
		return nil, err
	}

	// The unbind is always synchronous
	response := broker.UnbindResponse{}

	klog.V(4).Infof("broker: unbound %q", request.InstanceID)

	return &response, nil
}

func (b *Broker) Update(request *osb.UpdateInstanceRequest, _ *broker.RequestContext) (*broker.UpdateInstanceResponse, error) {
	// Not supported, do nothing

	response := broker.UpdateInstanceResponse{}
	if request.AcceptsIncomplete {
		response.Async = false // We do not currently accept asynchronous operations on update
	}

	return &response, nil
}

func (b *Broker) ValidateBrokerAPIVersion(version string) error {
	return nil
}
