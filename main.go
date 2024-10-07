package main

import (
	"log"

	"github.com/jiaqi/client-go-ingress/pkg"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// 1. config
	// 2. client
	// 3. informer
	// 4. add event handler
	// 5. informer.Start

	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		inClusterConfig, err := rest.InClusterConfig()
		if err != nil {
			log.Fatalln("cannot get config")
		}
		config = inClusterConfig
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalln("cannot get client")
	}

	// List and watch the default namespace
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 0, informers.WithNamespace("default"))
	serviceInformer := factory.Core().V1().Services()
	ingressInformer := factory.Networking().V1().Ingresses()

	controller := pkg.NewController(clientset, serviceInformer, ingressInformer)

	stopCh := make(chan struct{})
	factory.Start(stopCh)
	factory.WaitForCacheSync(stopCh)

	controller.Run(stopCh)
}
