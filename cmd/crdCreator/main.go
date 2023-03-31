package main

import (
	"context"
	"fmt"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const baseDir string = "shim/crds"

//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;watch;list;create

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	log := ctrl.Log.WithName("main")

	ctx := context.Background()

	log.Info("Setting up dynamic client")
	dynClient, err := dynamic.NewForConfig(config.GetConfigOrDie())
	if err != nil {
		log.Error(err, "error creating dynamic client")
		os.Exit(1)
	}

	crdFiles := []string{
		"storageclusters.ocs.openshift.io.yaml",
		"ocsinitializations.ocs.openshift.io.yaml",
		"noobaas.noobaa.io.yaml",
		"objectbucketclaims.objectbucket.io.yaml",
	}
	var templates [][]byte
	for _, f := range crdFiles {
		file, err := os.ReadFile(fmt.Sprintf("%s/%s", baseDir, f))
		if err != nil {
			log.Error(err, "unable to read crd templates")
			os.Exit(1)
		}
		templates = append(templates, file)
	}

	crdResource := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	crdKind := schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	}

	for _, template := range templates {
		obj := &unstructured.Unstructured{}
		dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		if _, _, err := dec.Decode(template, &crdKind, obj); err != nil {
			log.Error(err, "unable to decode YAML into unstructured struct")
			os.Exit(1)
		}

		if _, err := dynClient.Resource(crdResource).Create(ctx, obj, v1.CreateOptions{}); err == nil {
			log.Info("CRD Created", "Name", obj.GetName())
		} else if apierrors.IsAlreadyExists(err) {
			log.Info("CRD already exists", "Name", obj.GetName())
		} else {
			log.Error(err, "unable to create CRD", "Name", obj.GetName())
		}
	}
}
