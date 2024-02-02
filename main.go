package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var config *rest.Config
	var err error

	// Check if the program is running inside a cluster
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		config, err = rest.InClusterConfig()
	} else {
		// Uses kubeconfig file
		kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if err != nil {
		panic(err)
	}

	// Create a dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	// Define the GVR
	gvr := schema.GroupVersionResource{
		Group:    "spaas.smartnews.com", // Specify the CRD's group
		Version:  "v1alpha1",            // Specify the CRD's version
		Resource: "terraformplans",      // The plural name of the resource (lowercase)
	}

	// List all namespaces
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}

	for _, namespace := range namespaces.Items {
		name := namespace.Name
		fmt.Printf("Deleting terraformplan resources in %s namespace.\n", name)

		// List and delete resources in batches
		// Replace the batch size with your desired value
		batchSize := int64(100) // Example batch size

		listOpts := metav1.ListOptions{Limit: batchSize}
		for {
			resources, err := dynamicClient.Resource(gvr).Namespace(name).List(context.TODO(), listOpts)
			if err != nil {
				listOpts = metav1.ListOptions{Limit: batchSize}
				continue
				// fmt.Fprintf(os.Stderr, "Failed to list %s: %v\n", name, err)
			}

			var wg sync.WaitGroup

			for _, item := range resources.Items {
				wg.Add(1)                                           // Indicate that a goroutine is starting
				go processItem(dynamicClient, gvr, name, item, &wg) // Process each item in a separate goroutine
			}

			wg.Wait() // Wait for all goroutines to finish

			// Check if we have more items to fetch
			if len(resources.Items) == 0 || resources.GetContinue() == "" {
				break
			}

			listOpts.Continue = resources.GetContinue()
		}

		// Sleep to avoid overwhelming the API server, adjust as needed
		// time.Sleep(2 * time.Second)
	}
}

func processItem(dynamicClient *dynamic.DynamicClient, gvr schema.GroupVersionResource, name string, item unstructured.Unstructured, wg *sync.WaitGroup) {
	defer wg.Done() // Notify the WaitGroup that this goroutine is done

	// Perform the deletion
	if len(item.GetFinalizers()) > 0 {
		item, _ := dynamicClient.Resource(gvr).Namespace(name).Get(context.Background(), item.GetName(), metav1.GetOptions{})
		item.SetFinalizers(nil)
		if _, err := dynamicClient.Resource(gvr).Namespace(name).Update(context.Background(), item, metav1.UpdateOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to remove finalizer %s: %v\n", item.GetName(), err)
		} else {
			fmt.Printf("Remove finalizer  %s\n", item.GetName())
		}
	}

	err := dynamicClient.Resource(gvr).Namespace(name).Delete(context.Background(), item.GetName(), metav1.DeleteOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", item.GetName(), err)
	} else {
		fmt.Printf("Deleted %s\n", item.GetName())
	}
}
