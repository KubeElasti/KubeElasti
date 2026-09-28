package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	elastiv1alpha1 "truefoundry/elasti/operator/api/v1alpha1"
)

// The scale-down loop reads ElastiServices through the manager's cache
// instead of a fresh API LIST every cycle; the lister must still see every
// ElastiService the manager watches.
var _ = Describe("CachedElastiServiceLister", func() {
	ctx := context.Background()

	It("lists ElastiServices across namespaces", func() {
		for _, ns := range []string{"elasti-lister-a", "elasti-lister-b"} {
			err := k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
			if !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
			es := &elastiv1alpha1.ElastiService{
				ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns},
				Spec: elastiv1alpha1.ElastiServiceSpec{
					Service:        "app",
					ScaleTargetRef: elastiv1alpha1.ScaleTargetRef{APIVersion: "apps/v1", Kind: "Deployment", Name: "app"},
				},
			}
			Expect(k8sClient.Create(ctx, es)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, es) })
		}

		services, err := CachedElastiServiceLister(k8sClient)(ctx)
		Expect(err).NotTo(HaveOccurred())
		var keys []string
		for _, es := range services {
			keys = append(keys, es.Namespace+"/"+es.Name)
		}
		Expect(keys).To(ContainElements("elasti-lister-a/app", "elasti-lister-b/app"))
	})
})
