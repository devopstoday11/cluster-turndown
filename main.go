package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kubecost/kubecost-turndown/turndown"
	"github.com/kubecost/kubecost-turndown/turndown/provider"
	"github.com/kubecost/kubecost-turndown/turndown/strategy"

	"k8s.io/klog"
)

func runWebServer(scheduler *turndown.TurndownScheduler, manager turndown.TurndownManager, provider provider.ComputeProvider) {
	mux := http.NewServeMux()

	endpoints := turndown.NewTurndownEndpoints(scheduler, manager, provider)

	mux.HandleFunc("/schedule", endpoints.HandleStartSchedule)
	mux.HandleFunc("/cancel", endpoints.HandleCancelSchedule)
	mux.HandleFunc("/serviceKey", endpoints.HandleSetServiceKey)

	klog.Fatal(http.ListenAndServe(":9731", mux))
}

func initKubernetes() kubernetes.Interface {
	// Kubernetes API setup
	kc, err := rest.InClusterConfig()
	if err != nil {
		return nil
	}

	kubeClient, err := kubernetes.NewForConfig(kc)
	if err != nil {
		return nil
	}

	return kubeClient
}

// For now, we'll choose our strategy based on the provider, but functionally, there is
// no dependency.
func strategyForProvider(c kubernetes.Interface, p provider.ComputeProvider) (strategy.TurndownStrategy, error) {
	switch v := p.(type) {
	case *provider.GKEProvider:
		return strategy.NewMasterlessTurndownStrategy(c, p), nil
	case *provider.AWSProvider:
		return strategy.NewStandardTurndownStrategy(c, p), nil
	default:
		return nil, fmt.Errorf("No strategy available for: %+v", v)
	}
}

func main() {
	klog.InitFlags(nil)
	flag.Set("v", "5")
	flag.Parse()

	node := os.Getenv("NODE_NAME")
	klog.V(1).Infof("Running Kubecost Turndown on: %s", node)

	// Setup Components
	kubeClient := initKubernetes()
	scheduleStore := turndown.NewDiskScheduleStore("/var/configs/schedule.json")
	provider, err := provider.NewProvider(kubeClient)
	if err != nil {
		klog.V(1).Infof("Failed to determine provider: %s", err.Error())
		return
	}
	strategy, err := strategyForProvider(kubeClient, provider)
	if err != nil {
		klog.V(1).Infof("Failed to create strategy: %s", err.Error())
		return
	}
	manager := turndown.NewKubernetesTurndownManager(kubeClient, provider, strategy, node)
	scheduler := turndown.NewTurndownScheduler(manager, scheduleStore)

	runWebServer(scheduler, manager, provider)
}
