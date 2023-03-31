package main

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	tutils "github.com/red-hat-storage/managed-fusion-agent/testutils"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("CRD Creator Behaviour", func() {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	ctx := context.Background()

	testStorageClusterCRD := apiextensions.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "storageclusters.ocs.openshift.io",
		},
	}

	Context("reconcile()", Ordered, func() {
		When("there is no storage cluster CRD", func() {
			It("should create storage cluster CRD", func() {
				crd := testStorageClusterCRD.DeepCopy()
				fmt.Println(k8sClient.Get(ctx, tutils.GetResourceKey(crd), crd))
				// Expect(k8sClient.Get(ctx, tutils.GetResourceKey(crd), crd)).Should(
				// 	WithTransform(errors.IsNotFound, BeTrue()),
				// )
				main()
				crd = testStorageClusterCRD.DeepCopy()
				Expect(k8sClient.Get(ctx, tutils.GetResourceKey(crd), crd)).Should(Succeed())
				fmt.Println(crd)
			})
		})
	})
})
