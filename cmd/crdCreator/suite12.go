/*
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
package main

import (
	"io"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

const testNamespace = "default"

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "CRD Creator")
}

var _ = BeforeSuite(func() {
	done := make(chan interface{})

	var logFile io.Writer
	logFile, err := os.Create("/tmp/temp.log")
	Expect(err).ToNot(HaveOccurred())

	go func(logFile io.Writer) {
		GinkgoWriter.TeeTo(logFile)
		logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

		var err error
		testEnv = &envtest.Environment{}
		cfg, err = testEnv.Start()
		Expect(err).ToNot(HaveOccurred())
		Expect(cfg).ToNot(BeNil())

		scheme := runtime.NewScheme()

		err = clientgoscheme.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		err = apiextensionsv1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		// Client to be use by the test code, using a non cached client
		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred())
		Expect(k8sClient).ToNot(BeNil())

		close(done)
	}(logFile)
	Eventually(done, 60).Should(BeClosed())
})

var _ = AfterSuite(func() {
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
