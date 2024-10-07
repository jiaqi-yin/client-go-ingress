package pkg

import (
	"context"
	"reflect"
	"time"

	apicore "k8s.io/api/core/v1"
	apinetworking "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apismeta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	informerscore "k8s.io/client-go/informers/core/v1"
	informersnetworking "k8s.io/client-go/informers/networking/v1"
	"k8s.io/client-go/kubernetes"
	listerscore "k8s.io/client-go/listers/core/v1"
	listersnetworking "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const (
	workNum  = 5
	maxRetry = 10
)

type controller struct {
	client        kubernetes.Interface
	ingressLister listersnetworking.IngressLister
	serviceLister listerscore.ServiceLister
	queue         workqueue.RateLimitingInterface
}

func (c *controller) addService(obj interface{}) {
	c.enqueue(obj)
}

func (c *controller) updateService(oldObj interface{}, newObj interface{}) {
	// TODO: compare annotation
	if reflect.DeepEqual(oldObj, newObj) {
		return
	}

	c.enqueue(newObj)
}

func (c *controller) enqueue(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		runtime.HandleError(err)
	}

	c.queue.Add(key)
}

func (c *controller) deleteIngress(obj interface{}) {
	ingress := obj.(*apinetworking.Ingress)
	ownerReference := apismeta.GetControllerOf(ingress)

	if ownerReference == nil {
		return
	}

	if ownerReference.Kind != "Service" {
		return
	}

	c.queue.Add(ingress.Namespace + "/" + ingress.Name)
}

func (c *controller) Run(stopCh chan struct{}) {
	for i := 0; i < workNum; i++ {
		go wait.Until(c.worker, time.Minute, stopCh)
	}
	<-stopCh
}

func (c *controller) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *controller) processNextWorkItem() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	key := item.(string)

	err := c.syncService(key)
	if err != nil {
		c.handleError(key, err)
	}

	return true
}

func (c *controller) syncService(key string) error {
	namespaceKey, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	service, err := c.serviceLister.Services(namespaceKey).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// add and delete
	_, ok := service.GetAnnotations()["ingress/http"]
	ingress, err := c.ingressLister.Ingresses(namespaceKey).Get(name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if ok && errors.IsNotFound(err) {
		// add: has service but no ingress, then create ingress
		ig := c.constructIngress(service)
		_, err := c.client.NetworkingV1().Ingresses(namespaceKey).Create(context.TODO(), ig, apismeta.CreateOptions{})
		if err != nil {
			return err
		}
	} else if !ok && ingress != nil {
		// delete: has ingress but no service, then delete ingress
		err := c.client.NetworkingV1().Ingresses(namespaceKey).Delete(context.TODO(), name, apismeta.DeleteOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *controller) constructIngress(service *apicore.Service) *apinetworking.Ingress {
	ingressClassName := "nginx"
	pathType := apinetworking.PathTypePrefix

	return &apinetworking.Ingress{
		ObjectMeta: apismeta.ObjectMeta{
			Name:      service.Name,
			Namespace: service.Namespace,
			OwnerReferences: []apismeta.OwnerReference{
				*apismeta.NewControllerRef(service, apicore.SchemeGroupVersion.WithKind("Service")),
			},
		},
		Spec: apinetworking.IngressSpec{
			IngressClassName: &ingressClassName,
			Rules: []apinetworking.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: apinetworking.IngressRuleValue{
						HTTP: &apinetworking.HTTPIngressRuleValue{
							Paths: []apinetworking.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: apinetworking.IngressBackend{
										Service: &apinetworking.IngressServiceBackend{
											Name: service.Name,
											Port: apinetworking.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (c *controller) handleError(key string, err error) {
	if c.queue.NumRequeues(key) <= maxRetry {
		c.queue.AddRateLimited(key)
	}

	runtime.HandleError(err)
	c.queue.Forget(key)
}

func NewController(
	client kubernetes.Interface,
	serviceInformer informerscore.ServiceInformer,
	ingressInformer informersnetworking.IngressInformer,
) controller {
	c := controller{
		client:        client,
		ingressLister: ingressInformer.Lister(),
		serviceLister: serviceInformer.Lister(),
		queue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ingressManager"),
	}

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.addService,
		UpdateFunc: c.updateService,
	})

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.deleteIngress,
	})

	return c
}
