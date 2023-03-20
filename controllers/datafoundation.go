package controllers

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	v1alpha1 "github.com/red-hat-storage/managed-fusion-agent/api/v1alpha1"
	"github.com/red-hat-storage/managed-fusion-agent/templates"
	ocsv1 "github.com/red-hat-storage/ocs-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
)

const (
	enableMCGKey           = "enableMCG"
	usableCapacityInTiBKey = "usableCapacityInTiB"
	deviceSetName          = "default"
)

type dataFoundationSpec struct {
	usableCapacityInTiB     int
	onboardingValidationKey string
}

type dataFoundationReconciler struct {
	*ManagedFusionOfferingReconciler

	spec                          dataFoundationSpec
	onboardingValidationKeySecret *corev1.Secret
}

//+kubebuilder:rbac:groups=ocs.openshift.io,namespace=system,resources=storageclusters,verbs=get;list;watch;create;update;patch;delete

func dfSetupWatches(controllerBuilder *builder.Builder) {
	controllerBuilder.
		Owns(&corev1.Secret{}).
		Owns(&ocsv1.StorageCluster{})
}

func DFAddToScheme(scheme *runtime.Scheme) {
	utilruntime.Must(ocsv1.AddToScheme(scheme))
}

func dfReconcile(offeringReconciler *ManagedFusionOfferingReconciler, offering *v1alpha1.ManagedFusionOffering) error {
	r := dataFoundationReconciler{}
	r.initReconciler(offeringReconciler)

	if err := r.parseSpec(offering); err != nil {
		return err
	}
	if err := r.reconcileOnboardingValidationSecret(); err != nil {
		return err
	}
	if err := r.reconcileStorageCluster(); err != nil {
		return err
	}

	return nil
}

func (r *dataFoundationReconciler) initReconciler(offeringReconciler *ManagedFusionOfferingReconciler) {
	r.ManagedFusionOfferingReconciler = offeringReconciler

	r.onboardingValidationKeySecret = &corev1.Secret{}
	r.onboardingValidationKeySecret.Name = "onboarding-ticket-key"
	r.onboardingValidationKeySecret.Namespace = r.Namespace
}

func (r *dataFoundationReconciler) parseSpec(offering *v1alpha1.ManagedFusionOffering) error {
	r.Log.Info("Parsing ManagedFusionOffering Data Foundation spec")

	isValid := true
	var err error
	var usableCapacityInTiB int
	usableCapacityInTiBAsString, found := offering.Spec.Config["usableCapacityInTiB"]
	if !found {
		r.Log.Error(
			fmt.Errorf("missing field: usableCapacityInTiB"),
			"an error occurred while parsing ManagedFusionOffering Data Foundation spec",
		)
		isValid = false
	} else if usableCapacityInTiB, err = strconv.Atoi(usableCapacityInTiBAsString); err != nil {
		r.Log.Error(
			fmt.Errorf("error parsing usableCapacityInTib: %v", err),
			"an error occurred while parsing ManagedFusionOffering Data Foundation spec",
		)
		isValid = false
	}

	onboardingValidationKeyAsString, found := offering.Spec.Config["onboardingValidationKey"]
	if !found {
		r.Log.Error(
			fmt.Errorf("missing field: onboardingValidationKey"),
			"an error occurred while parsing ManagedFusionOffering Data Foundation spec",
		)
		isValid = false
	}

	if !isValid {
		r.Log.Info("parsing ManagedFusionOffering Data Foundation spec failed")
		return fmt.Errorf("invalid ManagedFusionOffering Data Foundation spec")
	}
	r.Log.Info("parsing ManagedFusionOffering Data Foundation spec completed successfuly")

	r.spec = dataFoundationSpec{
		usableCapacityInTiB:     usableCapacityInTiB,
		onboardingValidationKey: onboardingValidationKeyAsString,
	}
	return nil
}

func (r *dataFoundationReconciler) reconcileOnboardingValidationSecret() error {
	r.Log.Info("Reconciling onboardingValidationKey secret")

	_, err := ctrl.CreateOrUpdate(r.Ctx, r.Client, r.onboardingValidationKeySecret, func() error {
		if err := r.own(r.onboardingValidationKeySecret); err != nil {
			return err
		}
		onboardingValidationData := fmt.Sprintf(
			"-----BEGIN PUBLIC KEY-----\n%s\n-----END PUBLIC KEY-----",
			strings.TrimSpace(r.spec.onboardingValidationKey),
		)
		r.onboardingValidationKeySecret.Data = map[string][]byte{
			"key": []byte(onboardingValidationData),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update onboardingValidationKeySecret: %v", err)
	}
	return nil
}

func (r *dataFoundationReconciler) reconcileStorageCluster() error {
	r.Log.Info("Reconciling StorageCluster")

	storageCluster := &ocsv1.StorageCluster{}
	storageCluster.Name = "ocs-storagecluster"
	storageCluster.Namespace = r.managedFusionOffering.Namespace
	_, err := ctrl.CreateOrUpdate(r.Ctx, r.Client, storageCluster, func() error {
		if err := r.own(storageCluster); err != nil {
			return err
		}
		sizeAsString := r.managedFusionOffering.Spec.Config["usableCapacityInTiB"]

		// Setting hardcoded value here to force no MCG deployment
		enableMCGAsString := "false"
		if enableMCGRaw, exists := r.managedFusionOffering.Spec.Config[enableMCGKey]; exists {
			enableMCGAsString = enableMCGRaw
		}
		r.Log.Info("Requested add-on settings", usableCapacityInTiBKey, sizeAsString, enableMCGKey, enableMCGAsString)
		desiredSize, err := strconv.Atoi(sizeAsString)
		if err != nil {
			return fmt.Errorf("invalid storage cluster size value: %v", sizeAsString)
		}

		// Convert the desired size to the device set count based on the underlaying OSD size
		desiredDeviceSetCount := int(math.Ceil(float64(desiredSize) / templates.ProviderOSDSizeInTiB))

		// Get the storage device set count of the current storage cluster
		currDeviceSetCount := 0
		if desiredStorageDeviceSet := findStorageDeviceSet(storageCluster.Spec.StorageDeviceSets, deviceSetName); desiredStorageDeviceSet != nil {
			currDeviceSetCount = desiredStorageDeviceSet.Count
		}

		// Get the desired storage device set from storage cluster template
		sc := templates.ProviderStorageClusterTemplate.DeepCopy()
		var ds *ocsv1.StorageDeviceSet = nil
		if desiredStorageDeviceSet := findStorageDeviceSet(sc.Spec.StorageDeviceSets, deviceSetName); desiredStorageDeviceSet != nil {
			ds = desiredStorageDeviceSet
		}

		// Prevent downscaling by comparing count from secret and count from storage cluster
		setDeviceSetCount(r, ds, desiredDeviceSetCount, currDeviceSetCount)

		// Check and enable MCG in Storage Cluster spec
		mcgEnable, err := strconv.ParseBool(enableMCGAsString)
		if err != nil {
			return fmt.Errorf("invalid Enable MCG value: %v", enableMCGAsString)
		}
		if err := ensureMCGDeployment(r, sc, mcgEnable); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to create/update StorageCluster: %v", err)

	}

	return nil
}

func findStorageDeviceSet(storageDeviceSets []ocsv1.StorageDeviceSet, deviceSetName string) *ocsv1.StorageDeviceSet {
	for index := range storageDeviceSets {
		item := &storageDeviceSets[index]
		if item.Name == deviceSetName {
			return item
		}
	}
	return nil
}

func setDeviceSetCount(r *dataFoundationReconciler, deviceSet *ocsv1.StorageDeviceSet, desiredDeviceSetCount int, currDeviceSetCount int) {
	r.Log.Info("Setting storage device set count", "Current", currDeviceSetCount, "New", desiredDeviceSetCount)
	if currDeviceSetCount <= desiredDeviceSetCount {
		deviceSet.Count = desiredDeviceSetCount
	} else {
		r.Log.V(-1).Info("Requested storage device set count will result in downscaling, which is not supported. Skipping")
		deviceSet.Count = currDeviceSetCount
	}
}

func ensureMCGDeployment(r *dataFoundationReconciler, storageCluster *ocsv1.StorageCluster, mcgEnable bool) error {
	// Check and enable MCG in Storage Cluster spec
	if mcgEnable {
		r.Log.Info("Enabling Multi Cloud Gateway")
		storageCluster.Spec.MultiCloudGateway.ReconcileStrategy = "manage"
	} else if storageCluster.Spec.MultiCloudGateway.ReconcileStrategy == "manage" {
		r.Log.V(-1).Info("Trying to disable Multi Cloud Gateway, Invalid operation")
	}
	return nil
}
