/*
Copyright 2026.

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

package v1alpha1

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpnsv1alpha1 "github.com/njferrant/k8s-tmpns-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
)

var defaultUser = &tmpnsv1alpha1.ResourceOwner{
	Username: "john.smith@example.com",
	UID:      string(uuid.NewUUID()),
}

var _ = Describe("TempNamespace Webhook", func() {
	var (
		obj       *tmpnsv1alpha1.TempNamespace
		oldObj    *tmpnsv1alpha1.TempNamespace
		defaulter TempNamespaceDefaulter
	)

	BeforeEach(func() {
		obj = &tmpnsv1alpha1.TempNamespace{}
		oldObj = &tmpnsv1alpha1.TempNamespace{}
		defaulter = TempNamespaceDefaulter{}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating a TempNamespace CR under Defaulting Webhook", func() {
		It("sets the owner on create", func() {
			obj := &tmpnsv1alpha1.TempNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "create-owner-test"},
				Spec:       tmpnsv1alpha1.TempNamespaceSpec{Duration: "2h"},
			}
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			Expect(obj.Spec.Owner).NotTo(BeNil())
			Expect(obj.Spec.Owner.Username).NotTo(BeEmpty())
			Expect(obj.Spec.Owner).To(Equal(defaultUser))
		})
		It("sets createdAt", func() {
			obj := &tmpnsv1alpha1.TempNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "created-at-test"},
				Spec: tmpnsv1alpha1.TempNamespaceSpec{
					Duration: "2h",
					// Provide an incorrect CreatedAt to ensure the mutating webhook
					// overwrites it with actual creation time.
					CreatedAt: &metav1.Time{Time: time.Unix(0, 0)},
				},
			}
			before := time.Now().Truncate(time.Second)
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			after := time.Now()
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			Expect(obj.Spec.CreatedAt).NotTo(BeNil())
			Expect(obj.Spec.CreatedAt.Time).To(BeTemporally(">=", before))
			Expect(obj.Spec.CreatedAt.Time).To(BeTemporally("<=", after))
		})
		DescribeTable("sets expiresAt from duration",
			func(name, duration string, expected func(created time.Time) time.Time) {
				obj := &tmpnsv1alpha1.TempNamespace{
					ObjectMeta: metav1.ObjectMeta{Name: name},
					Spec:       tmpnsv1alpha1.TempNamespaceSpec{Duration: duration},
				}
				Expect(k8sClient.Create(ctx, obj)).To(Succeed())
				DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

				Expect(obj.Spec.ExpiresAt).NotTo(BeNil())
				Expect(obj.Spec.Duration).To(BeEmpty())
				Expect(obj.Spec.ExpiresAt.Time).To(Equal(expected(obj.Spec.CreatedAt.Time)))
			},
			Entry("hours", "expires-at-hours-test", "2h", func(t time.Time) time.Time { return t.Add(2 * time.Hour) }),
			Entry("days", "expires-at-days-test", "2d", func(t time.Time) time.Time { return t.AddDate(0, 0, 2) }),
			Entry("weeks", "expires-at-weeks-test", "2w", func(t time.Time) time.Time { return t.AddDate(0, 0, 14) }),
			Entry("months", "expires-at-months-test", "2m", func(t time.Time) time.Time { return t.AddDate(0, 2, 0) }),
		)
		It("sets invalid duration", func() {
			obj := &tmpnsv1alpha1.TempNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "expires-at-hours-test"},
				Spec:       tmpnsv1alpha1.TempNamespaceSpec{Duration: "1.5d"},
			}
			err := k8sClient.Create(ctx, obj)
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.duration in body should match"))
			Expect(obj.Spec.ExpiresAt).To(BeNil())
			Expect(obj.Spec.Duration).To(Equal("1.5d"))
		})
		// PLACEHOLDER: have another user modify and try to change the owner
	})
	Context("When updating a TempNamespace CR under Defaulting Webhook", func() {
		It("discards duration", func() {
			obj := &tmpnsv1alpha1.TempNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "update-duration-test"},
				Spec: tmpnsv1alpha1.TempNamespaceSpec{
					Duration: "2h",
				},
			}
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
			before := obj.DeepCopy()

			obj.Spec.Duration = "1w"
			Expect(k8sClient.Update(ctx, obj)).To(Succeed())
			Expect(obj.Spec.Duration).To(BeEmpty())
			Expect(obj.Spec.CreatedAt).To(Equal(before.Spec.CreatedAt))
			Expect(obj.Spec.ExpiresAt).To(Equal(before.Spec.ExpiresAt))
		})
	})

})
