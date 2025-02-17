package cluster_manager

import (
	"context"
	"github.com/kurtosis-tech/kardinal/libs/manager-kontrol-api/api/golang/types"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/samber/lo"
	"github.com/sirupsen/logrus"
	istio "istio.io/api/networking/v1alpha3"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kardinal.kontrol/kardinal-manager/topology"
)

const (
	listOptionsTimeoutSeconds       int64 = 10
	fieldManager                          = "kardinal-manager"
	deleteOptionsGracePeriodSeconds int64 = 0
	istioLabel                            = "istio-injection"
	enabledIstioValue                     = "enabled"
)

var (
	globalListOptions = metav1.ListOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		LabelSelector:        "",
		FieldSelector:        "",
		Watch:                false,
		AllowWatchBookmarks:  false,
		ResourceVersion:      "",
		ResourceVersionMatch: "",
		TimeoutSeconds:       int64Ptr(listOptionsTimeoutSeconds),
		Limit:                0,
		Continue:             "",
		SendInitialEvents:    nil,
	}

	globalGetOptions = metav1.GetOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		ResourceVersion: "",
	}

	globalCreateOptions = metav1.CreateOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		DryRun: nil,
		// We need every object to have this field manager so that the Kurtosis objects can all seamlessly modify Kubernetes resources
		FieldManager:    fieldManager,
		FieldValidation: "",
	}

	globalUpdateOptions = metav1.UpdateOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		DryRun: nil,
		// We need every object to have this field manager so that the Kurtosis objects can all seamlessly modify Kubernetes resources
		FieldManager:    fieldManager,
		FieldValidation: "",
	}

	globalDeletePolicy = metav1.DeletePropagationForeground

	globalDeleteOptions = metav1.DeleteOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		GracePeriodSeconds: int64Ptr(deleteOptionsGracePeriodSeconds),
		Preconditions:      nil,
		OrphanDependents:   nil,
		PropagationPolicy:  &globalDeletePolicy,
		DryRun:             nil,
	}
)

type ClusterManager struct {
	kubernetesClient *kubernetesClient
	istioClient      *istioClient
}

func NewClusterManager(kubernetesClient *kubernetesClient, istioClient *istioClient) *ClusterManager {
	return &ClusterManager{kubernetesClient: kubernetesClient, istioClient: istioClient}
}

func (manager *ClusterManager) GetVirtualServices(ctx context.Context, namespace string) ([]*v1alpha3.VirtualService, error) {
	virtServiceClient := manager.istioClient.clientSet.NetworkingV1alpha3().VirtualServices(namespace)

	virtualServiceList, err := virtServiceClient.List(ctx, globalListOptions)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred retrieving virtual services from IstIo client.")
	}
	return virtualServiceList.Items, nil
}

func (manager *ClusterManager) GetVirtualService(ctx context.Context, namespace string, name string) (*v1alpha3.VirtualService, error) {
	virtServiceClient := manager.istioClient.clientSet.NetworkingV1alpha3().VirtualServices(namespace)

	virtualService, err := virtServiceClient.Get(ctx, name, globalGetOptions)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred retrieving virtual service '%s' from IstIo client", name)
	}
	return virtualService, nil
}

func (manager *ClusterManager) GetDestinationRules(ctx context.Context, namespace string) ([]*v1alpha3.DestinationRule, error) {
	destRuleClient := manager.istioClient.clientSet.NetworkingV1alpha3().DestinationRules(namespace)

	destinationRules, err := destRuleClient.List(ctx, globalListOptions)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred retrieving destination rules.")
	}
	return destinationRules.Items, nil
}

func (manager *ClusterManager) GetDestinationRule(ctx context.Context, namespace string, rule string) (*v1alpha3.DestinationRule, error) {
	destRuleClient := manager.istioClient.clientSet.NetworkingV1alpha3().DestinationRules(namespace)

	destinationRule, err := destRuleClient.Get(ctx, rule, globalGetOptions)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred retrieving destination rule '%s' from IstIo client", rule)
	}
	return destinationRule, nil
}

// how to expose API to configure ordering of routing rule? https://istio.io/latest/docs/concepts/traffic-management/#routing-rule-precedence
func (manager *ClusterManager) AddRoutingRule(ctx context.Context, namespace string, vsName string, routingRule *istio.HTTPRoute) error {
	virtServiceClient := manager.istioClient.clientSet.NetworkingV1alpha3().VirtualServices(namespace)

	vs, err := virtServiceClient.Get(ctx, vsName, globalGetOptions)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred retrieving virtual service '%s'", vsName)
	}
	// always prepend routing rules due to routing rule precedence
	vs.Spec.Http = append([]*istio.HTTPRoute{routingRule}, vs.Spec.Http...)
	_, err = virtServiceClient.Update(ctx, vs, metav1.UpdateOptions{})
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred updating virtual service '%s' with routing rule: %v", vsName, routingRule)
	}
	return nil
}

func (manager *ClusterManager) AddSubset(ctx context.Context, namespace string, drName string, subset *istio.Subset) error {
	destRuleClient := manager.istioClient.clientSet.NetworkingV1alpha3().DestinationRules(namespace)

	dr, err := destRuleClient.Get(ctx, drName, globalGetOptions)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred retrieving destination rule '%s'", drName)
	}
	// if there already exists a subset for the same, just update it
	shouldAddNewSubset := true
	for _, s := range dr.Spec.Subsets {
		if s.Name == subset.Name {
			s = subset
			shouldAddNewSubset = false
		}
	}
	if shouldAddNewSubset {
		dr.Spec.Subsets = append(dr.Spec.Subsets, subset)
	}
	_, err = destRuleClient.Update(ctx, dr, metav1.UpdateOptions{})
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred updating destination rule '%s' with subset: %v", drName, subset)
	}
	return nil
}

func (manager *ClusterManager) GetTopologyForNameSpace(namespace string) (map[string]*topology.Node, error) {
	return manager.istioClient.topologyManager.FetchTopology(namespace)
}

func (manager *ClusterManager) ApplyClusterResources(ctx context.Context, clusterResources *types.ClusterResources) error {

	if !isValid(clusterResources) {
		logrus.Debugf("the received cluster resources is not valid, nothing to apply.")
		return nil
	}

	allNSs := [][]string{
		lo.Uniq(lo.Map(*clusterResources.Services, func(item corev1.Service, _ int) string { return item.Namespace })),
		lo.Uniq(lo.Map(*clusterResources.Deployments, func(item appsv1.Deployment, _ int) string { return item.Namespace })),
		lo.Uniq(lo.Map(*clusterResources.VirtualServices, func(item v1alpha3.VirtualService, _ int) string { return item.Namespace })),
		lo.Uniq(lo.Map(*clusterResources.DestinationRules, func(item v1alpha3.DestinationRule, _ int) string { return item.Namespace })),
		{clusterResources.Gateway.Namespace},
	}

	uniqueNamespaces := lo.Uniq(lo.Flatten(allNSs))

	for _, namespace := range uniqueNamespaces {
		if err := manager.ensureNamespace(ctx, namespace); err != nil {
			return stacktrace.Propagate(err, "An error occurred while creating or updating cluster namespace '%s'", namespace)
		}
	}

	for _, service := range *clusterResources.Services {
		if err := manager.createOrUpdateService(ctx, &service); err != nil {
			return stacktrace.Propagate(err, "An error occurred while creating or updating service '%s'", service.GetName())
		}
	}

	for _, deployment := range *clusterResources.Deployments {
		if err := manager.createOrUpdateDeployment(ctx, &deployment); err != nil {
			return stacktrace.Propagate(err, "An error occurred while creating or updating deployment '%s'", deployment.GetName())
		}
	}

	for _, virtualService := range *clusterResources.VirtualServices {
		if err := manager.createOrUpdateVirtualService(ctx, &virtualService); err != nil {
			return stacktrace.Propagate(err, "An error occurred while creating or updating virtual service '%s'", virtualService.GetName())
		}
	}

	for _, destinationRule := range *clusterResources.DestinationRules {
		if err := manager.createOrUpdateDestinationRule(ctx, &destinationRule); err != nil {
			return stacktrace.Propagate(err, "An error occurred while creating or updating destination rule '%s'", destinationRule.GetName())
		}
	}

	if err := manager.createOrUpdateGateway(ctx, clusterResources.Gateway); err != nil {
		return stacktrace.Propagate(err, "An error occurred while creating or updating the cluster gateway")
	}

	return nil
}

func (manager *ClusterManager) CleanUpClusterResources(ctx context.Context, clusterResources *types.ClusterResources) error {

	if !isValid(clusterResources) {
		logrus.Debugf("the received cluster resources is not valid, nothing to clean up.")
		return nil
	}

	// Clean up services
	servicesByNS := lo.GroupBy(*clusterResources.Services, func(item corev1.Service) string {
		return item.Namespace
	})
	for namespace, services := range servicesByNS {
		if err := manager.cleanUpServicesInNamespace(ctx, namespace, services); err != nil {
			return stacktrace.Propagate(err, "An error occurred cleaning up services '%+v' in namespace '%s'", services, namespace)
		}
	}

	// Clean up deployments
	deploymentsByNS := lo.GroupBy(*clusterResources.Deployments, func(item appsv1.Deployment) string { return item.Namespace })
	for namespace, deployments := range deploymentsByNS {
		if err := manager.cleanUpDeploymentsInNamespace(ctx, namespace, deployments); err != nil {
			return stacktrace.Propagate(err, "An error occurred cleaning up deployments '%+v' in namespace '%s'", deployments, namespace)
		}
	}

	// Clean up virtual services
	virtualServicesByNS := lo.GroupBy(*clusterResources.VirtualServices, func(item v1alpha3.VirtualService) string { return item.Namespace })
	for namespace, virtualServices := range virtualServicesByNS {
		if err := manager.cleanUpVirtualServicesInNamespace(ctx, namespace, virtualServices); err != nil {
			return stacktrace.Propagate(err, "An error occurred cleaning up virtual services '%+v' in namespace '%s'", virtualServices, namespace)
		}
	}

	// Clean up destination rules
	destinationRulesByNS := lo.GroupBy(*clusterResources.DestinationRules, func(item v1alpha3.DestinationRule) string {
		return item.Namespace
	})
	for namespace, destinationRules := range destinationRulesByNS {
		if err := manager.cleanUpDestinationRulesInNamespace(ctx, namespace, destinationRules); err != nil {
			return stacktrace.Propagate(err, "An error occurred cleaning up destination rules '%+v' in namespace '%s'", destinationRules, namespace)
		}
	}

	// Clean up gateway
	gatewaysByNs := map[string][]v1alpha3.Gateway{
		clusterResources.Gateway.GetNamespace(): {*clusterResources.Gateway},
	}
	for namespace, gateways := range gatewaysByNs {
		if err := manager.cleanUpGatewaysInNamespace(ctx, namespace, gateways); err != nil {
			return stacktrace.Propagate(err, "An error occurred cleaning up gateways '%+v' in namespace '%s'", gateways, namespace)
		}
	}

	return nil
}

func (manager *ClusterManager) ensureNamespace(ctx context.Context, name string) error {

	existingNamespace, err := manager.kubernetesClient.clientSet.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil && existingNamespace != nil {
		value, found := existingNamespace.Labels[istioLabel]
		if !found || value != enabledIstioValue {
			existingNamespace.Labels[istioLabel] = enabledIstioValue
			manager.kubernetesClient.clientSet.CoreV1().Namespaces().Update(ctx, existingNamespace, globalUpdateOptions)
		}
	} else {
		newNamespace := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					istioLabel: enabledIstioValue,
				},
			},
		}
		_, err = manager.kubernetesClient.clientSet.CoreV1().Namespaces().Create(ctx, &newNamespace, globalCreateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to create Namespace: %s", name)
		}
	}

	return nil
}

func (manager *ClusterManager) createOrUpdateService(ctx context.Context, service *corev1.Service) error {
	serviceClient := manager.kubernetesClient.clientSet.CoreV1().Services(service.Namespace)
	existingService, err := serviceClient.Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		// Resource does not exist, create new one
		_, err = serviceClient.Create(ctx, service, globalCreateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to create service: %s", service.GetName())
		}
	} else {
		// Update the resource version to the latest before updating
		service.ResourceVersion = existingService.ResourceVersion
		_, err = serviceClient.Update(ctx, service, globalUpdateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to update service: %s", service.GetName())
		}
	}

	return nil
}

func (manager *ClusterManager) createOrUpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	deploymentClient := manager.kubernetesClient.clientSet.AppsV1().Deployments(deployment.Namespace)
	existingDeployment, err := deploymentClient.Get(ctx, deployment.Name, metav1.GetOptions{})
	if err != nil {
		_, err = deploymentClient.Create(ctx, deployment, globalCreateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to create deployment: %s", deployment.GetName())
		}
	} else {
		deployment.ResourceVersion = existingDeployment.ResourceVersion
		_, err = deploymentClient.Update(ctx, deployment, globalUpdateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to update deployment: %s", deployment.GetName())
		}
	}

	return nil
}

func (manager *ClusterManager) createOrUpdateVirtualService(ctx context.Context, virtualService *v1alpha3.VirtualService) error {

	virtServiceClient := manager.istioClient.clientSet.NetworkingV1alpha3().VirtualServices(virtualService.GetNamespace())

	existingVirtService, err := virtServiceClient.Get(ctx, virtualService.Name, metav1.GetOptions{})
	if err != nil {
		_, err = virtServiceClient.Create(ctx, virtualService, globalCreateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to create virtual service: %s", virtualService.GetName())
		}
	} else {
		virtualService.ResourceVersion = existingVirtService.ResourceVersion
		_, err = virtServiceClient.Update(ctx, virtualService, globalUpdateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to update virtual service: %s", virtualService.GetName())
		}
	}

	return nil
}

func (manager *ClusterManager) createOrUpdateDestinationRule(ctx context.Context, destinationRule *v1alpha3.DestinationRule) error {

	destRuleClient := manager.istioClient.clientSet.NetworkingV1alpha3().DestinationRules(destinationRule.GetNamespace())

	existingDestRule, err := destRuleClient.Get(ctx, destinationRule.Name, metav1.GetOptions{})
	if err != nil {
		_, err = destRuleClient.Create(ctx, destinationRule, globalCreateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to create destination rule: %s", destinationRule.GetName())
		}
	} else {
		destinationRule.ResourceVersion = existingDestRule.ResourceVersion
		_, err = destRuleClient.Update(ctx, destinationRule, globalUpdateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to update destination rule: %s", destinationRule.GetName())
		}
	}

	return nil
}

func (manager *ClusterManager) createOrUpdateGateway(ctx context.Context, gateway *v1alpha3.Gateway) error {

	gatewayClient := manager.istioClient.clientSet.NetworkingV1alpha3().Gateways(gateway.GetNamespace())
	existingGateway, err := gatewayClient.Get(ctx, gateway.Name, metav1.GetOptions{})
	if err != nil {
		_, err = gatewayClient.Create(ctx, gateway, globalCreateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to create gateway: %s", gateway.GetName())
		}
	} else {
		gateway.ResourceVersion = existingGateway.ResourceVersion
		_, err = gatewayClient.Update(ctx, gateway, globalUpdateOptions)
		if err != nil {
			return stacktrace.Propagate(err, "Failed to update gateway: %s", gateway.GetName())
		}
	}

	return nil
}

func (manager *ClusterManager) cleanUpServicesInNamespace(ctx context.Context, namespace string, servicesToKeep []corev1.Service) error {
	serviceClient := manager.kubernetesClient.clientSet.CoreV1().Services(namespace)
	allServices, err := serviceClient.List(ctx, globalListOptions)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to list services in namespace %s", namespace)
	}
	for _, service := range allServices.Items {
		_, exists := lo.Find(servicesToKeep, func(item corev1.Service) bool { return item.Name == service.Name })
		if !exists {
			err = serviceClient.Delete(ctx, service.Name, globalDeleteOptions)
			if err != nil {
				return stacktrace.Propagate(err, "Failed to delete service %s", service.GetName())
			}
		}
	}
	return nil
}

func (manager *ClusterManager) cleanUpDeploymentsInNamespace(ctx context.Context, namespace string, deploymentsToKeep []appsv1.Deployment) error {
	deploymentClient := manager.kubernetesClient.clientSet.AppsV1().Deployments(namespace)
	allDeployments, err := deploymentClient.List(ctx, globalListOptions)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to list deployments in namespace %s", namespace)
	}
	for _, deployment := range allDeployments.Items {
		_, exists := lo.Find(deploymentsToKeep, func(item appsv1.Deployment) bool { return item.Name == deployment.Name })
		if !exists {
			err = deploymentClient.Delete(ctx, deployment.Name, globalDeleteOptions)
			if err != nil {
				return stacktrace.Propagate(err, "Failed to delete deployment %s", deployment.GetName())
			}
		}
	}
	return nil
}

func (manager *ClusterManager) cleanUpVirtualServicesInNamespace(ctx context.Context, namespace string, virtualServicesToKeep []v1alpha3.VirtualService) error {

	virtServiceClient := manager.istioClient.clientSet.NetworkingV1alpha3().VirtualServices(namespace)
	allVirtServices, err := virtServiceClient.List(ctx, globalListOptions)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to list virtual services in namespace %s", namespace)
	}
	for _, virtService := range allVirtServices.Items {
		_, exists := lo.Find(virtualServicesToKeep, func(item v1alpha3.VirtualService) bool { return item.Name == virtService.Name })
		if !exists {
			err = virtServiceClient.Delete(ctx, virtService.Name, globalDeleteOptions)
			if err != nil {
				return stacktrace.Propagate(err, "Failed to delete virtual service %s", virtService.GetName())
			}
		}
	}

	return nil
}

func (manager *ClusterManager) cleanUpDestinationRulesInNamespace(ctx context.Context, namespace string, destinationRulesToKeep []v1alpha3.DestinationRule) error {

	destRuleClient := manager.istioClient.clientSet.NetworkingV1alpha3().DestinationRules(namespace)
	allDestRules, err := destRuleClient.List(ctx, globalListOptions)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to list destination rules in namespace %s", namespace)
	}
	for _, destRule := range allDestRules.Items {
		_, exists := lo.Find(destinationRulesToKeep, func(item v1alpha3.DestinationRule) bool { return item.Name == destRule.Name })
		if !exists {
			err = destRuleClient.Delete(ctx, destRule.Name, globalDeleteOptions)
			if err != nil {
				return stacktrace.Propagate(err, "Failed to delete destination rule %s", destRule.GetName())
			}
		}
	}

	return nil
}

func (manager *ClusterManager) cleanUpGatewaysInNamespace(ctx context.Context, namespace string, gatewaysToKeep []v1alpha3.Gateway) error {

	gatewayClient := manager.istioClient.clientSet.NetworkingV1alpha3().Gateways(namespace)
	allGateways, err := gatewayClient.List(ctx, globalListOptions)
	if err != nil {
		return stacktrace.Propagate(err, "Failed to list gateways in namespace %s", namespace)
	}
	for _, gateway := range allGateways.Items {
		_, exists := lo.Find(gatewaysToKeep, func(item v1alpha3.Gateway) bool { return item.Name == gateway.Name })
		if !exists {
			err = gatewayClient.Delete(ctx, gateway.Name, globalDeleteOptions)
			if err != nil {
				return stacktrace.Propagate(err, "Failed to delete gateway %s", gateway.GetName())
			}
		}
	}

	return nil
}

func int64Ptr(i int64) *int64 { return &i }

func isValid(clusterResources *types.ClusterResources) bool {
	if clusterResources == nil {
		logrus.Debugf("cluster resources is nil.")
		return false
	}

	if clusterResources.Gateway == nil &&
		clusterResources.Deployments == nil &&
		clusterResources.DestinationRules == nil &&
		clusterResources.Services == nil &&
		clusterResources.VirtualServices == nil {
		logrus.Debugf("cluster resources is empty.")
		return false
	}

	return true
}
