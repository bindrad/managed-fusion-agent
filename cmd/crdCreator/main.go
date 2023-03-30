package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const baseDir string = "shim/crds"

//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;watch;list;create

func main() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	log := ctrl.Log.WithName("main")

	ctx := context.Background()

	log.Info("Setting up k8s client")
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	k8sClient, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "error creating client")
		os.Exit(1)
	}

	crdFiles := []string{
		"storageclusters.ocs.openshift.io.yaml",
		"ocsinitializations.ocs.openshift.io.yaml",
		"noobaas.noobaa.io.yaml",
	}

	var templates [][]byte
	for _, f := range crdFiles {
		file, err := ioutil.ReadFile(fmt.Sprintf("%s/%s", baseDir, f))
		if err != nil {
			log.Error(err, "unable to read crd templates")
			os.Exit(1)
		}
		templates = append(templates, file)
	}

	for _, template := range templates {
		obj := ObjectFromTemplate(template, scheme).(*apiextensionsv1.CustomResourceDefinition)
		if err := CreateIfNotExists(k8sClient, ctx, obj); err != nil {
			log.Error(err, "unable to create crd:", "Name", obj.GetName())
		}
	}
}

func CreateIfNotExists(k8sClient client.Client, ctx context.Context, obj client.Object) error {
	log := ctrl.Log.WithName("CreateCRD")
	key := client.ObjectKeyFromObject(obj)
	if err := k8sClient.Get(ctx, key, obj); err == nil {
		log.Info(fmt.Sprintf("%s CRD already exists", obj.GetName()))
	} else if apierrors.IsNotFound(err) {
		log.Info("Creating CRD", "Name", obj.GetName())
		if err := k8sClient.Create(ctx, obj); err != nil {
			return fmt.Errorf("unable to create the object: %v", err)
		}
	} else {
		return fmt.Errorf("unable to get the object: %v", err)
	}
	return nil
}

// ObjectFromTemplate returns a runtime object based on a text yaml/json string template
func ObjectFromTemplate(text []byte, scheme *runtime.Scheme) runtime.Object {
	// Decode text (yaml/json) to kube api object
	deserializer := serializer.NewCodecFactory(scheme).UniversalDeserializer()
	obj, group, err := deserializer.Decode(text, nil, nil)
	if err != nil {
		panic(err)
	}
	obj.GetObjectKind().SetGroupVersionKind(*group)
	return obj
}
