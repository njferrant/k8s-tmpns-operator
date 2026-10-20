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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	// "k8s.io/apimachinery/pkg/api/errors"
	tmpnsv1alpha1 "github.com/njferrant/k8s-tmpns-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("TempNamespace Controller", func() {
	var (
		ctx context.Context
		r   *TempNamespaceReconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		r = &TempNamespaceReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("When the TempNamespace doesn't exist", func() {
		It("should ignore a missing resource", func() {
			got, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: "does-not-exist",
				},
			})
			want := reconcile.Result{Requeue: false, RequeueAfter: 0, Priority: nil}
			Expect(err).NotTo(HaveOccurred())
			// could just do Expect(got.IsZero()).To(BeTrue()), but lets be more explicit and define 'want'
			Expect(got).To(Equal(want))
		})
	})

	Context("When the TempNamespace has expired", func() {
		It("Deletes an expired TempNamespace", func() {
			createdAt, expiresAt := createdAndExpiredMeta(
				time.Now().Add(-2*time.Hour),
				time.Hour,
			)

			tn := &tmpnsv1alpha1.TempNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "expired-resource"},
				Spec: tmpnsv1alpha1.TempNamespaceSpec{
					Owner:     &tmpnsv1alpha1.ResourceOwner{Username: "john.smith@example.com"},
					CreatedAt: &createdAt,
					ExpiresAt: &expiresAt,
				},
			}

			Expect(k8sClient.Create(ctx, tn)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, tn))).To(Succeed())
			})

			_, err := r.Reconcile(ctx, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(tn),
			})
			Expect(err).NotTo(HaveOccurred())

			// got := &tmpnsv1alpha1.TempNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(tn), tn)).
				To(MatchError(ContainSubstring("not found")))

		})
	})

	Context("When the TempNamespace has not expired", func() {
		var tn *tmpnsv1alpha1.TempNamespace
		name := "test-resource"
		owner := "john.smith@example.com"
		reconcileOnce := func() (reconcile.Result, error) {
			return r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKey{Name: name}})
		}

		BeforeEach(func() {
			createdAt, expiresAt := createdAndExpiredMeta(time.Now(), time.Hour)
			tn = &tmpnsv1alpha1.TempNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Spec: tmpnsv1alpha1.TempNamespaceSpec{
					Owner:     &tmpnsv1alpha1.ResourceOwner{Username: owner, UID: "7361ad47-471d-47d0-8cdc-8c520534bc82"},
					CreatedAt: &createdAt,
					ExpiresAt: &expiresAt,
				},
			}
			Expect(k8sClient.Create(ctx, tn)).To(Succeed())
			DeferCleanup(func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, tn))).To(Succeed())
				deleteControlledBy(ctx, &rbacv1.RoleBindingList{}, tn.UID)
				deleteControlledBy(ctx, &corev1.NamespaceList{}, tn.UID)
			})
		})
		It("Creates a labeled namespace controlled by TempNamespace", func() {
			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			assertNamespace(ctx, name, tn)
		})
		It("Gives the owner admin in the namespace through a RoleBinding", func() {
			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			assertRoleBinding(ctx, name, name+"-admin", owner, tn)

		})
		It("Recreates namespace and rolebinding if deleted", func() {
			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			By("Deleting the associated rolebinding and namespace")
			deleteControlledBy(ctx, &rbacv1.RoleBindingList{}, tn.UID)
			deleteControlledBy(ctx, &corev1.NamespaceList{}, tn.UID)

			By("Reconciling the state again")
			_, err = reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			assertRoleBinding(ctx, name, name+"-admin", owner, tn)
			assertNamespace(ctx, name, tn)
		})
		It("Restores namespace managed label if changed", func() {
			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			By("Changing the label value")
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, ns)).To(Succeed())
			ns.ObjectMeta.Labels["tmpns.io/managed"] = "false"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			By("reconciling the state again")
			_, err = reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			assertNamespace(ctx, name, tn)
		})
		It("Restores RoleBinding subjects to the TempNamespace owner", func() {
			_, err := reconcileOnce()
			Expect(err).NotTo(HaveOccurred())

			By("Changing the RoleBinding subject")
			rb := &rbacv1.RoleBinding{}
			key := client.ObjectKey{
				Namespace: name,
				Name:      name + "-admin",
			}
			Expect(k8sClient.Get(ctx, key, rb)).To(Succeed())
			rb.Subjects = append(rb.Subjects, rbacv1.Subject{
				Kind:     rbacv1.UserKind,
				APIGroup: rbacv1.GroupName,
				Name:     "unexpected-user",
			})
			Expect(k8sClient.Update(ctx, rb)).To(Succeed())

			By("Reconciling the state again")
			_, err = reconcileOnce()
			Expect(err).NotTo(HaveOccurred())
			assertRoleBinding(ctx, name, name+"-admin", owner, tn)
		})
	})
})

func createdAndExpiredMeta(createdAt time.Time, duration time.Duration) (metav1.Time, metav1.Time) {
	GinkgoHelper()

	expiresAt := createdAt.Add(duration)
	return metav1.Time{Time: createdAt}, metav1.Time{Time: expiresAt}
}

// deleteControlledBy is to make up for envtest's lack of a garbage collector. Therefore
// we are doing our own cleanup of child objects given an owner UID. Now there is an
// assumption here that the controller is properly setting an owner UID, but i can't think of
// a better way at the moment.
func deleteControlledBy(ctx context.Context, list client.ObjectList, ownerUID types.UID) {
	GinkgoHelper()

	Expect(k8sClient.List(ctx, list)).To(Succeed())
	items, err := meta.ExtractList(list)
	Expect(err).NotTo(HaveOccurred())

	for _, item := range items {
		obj := item.(client.Object)
		if ref := metav1.GetControllerOf(obj); ref != nil && ref.UID == ownerUID {
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, obj))).To(Succeed())
			// envtest also lacks a namespace controller, so we have to clear the finalizer
			// for it ot actually terminate. We re-fetch the namespace as the Delete method
			// modified the object, and without the latest copy we would get a conflict
			// when trying to update the finalizer.
			if _, ok := obj.(*corev1.Namespace); ok {
				ns := &corev1.Namespace{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), ns); err == nil {
					ns.Spec.Finalizers = nil
					Expect(k8sClient.SubResource("finalize").Update(ctx, ns)).To(Succeed())
				}
			}
		}
	}
}

func assertNamespace(ctx context.Context, name string, owningObj client.Object) {
	GinkgoHelper()

	ns := &corev1.Namespace{}
	Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, ns)).To(Succeed(),
		"Namespace %q should exist", name)

	key, value := "tmpns.io/managed", "true"
	Expect(ns.ObjectMeta.Labels).To(HaveKeyWithValue(key, value),
		"Namespace %q should have label %q=%q", name, key, value)

	ref := ns.GetOwnerReferences()
	Expect(ref).NotTo(BeNil(),
		"Namespace %q should have an owner reference", name)

	Expect(metav1.IsControlledBy(ns, owningObj)).To(BeTrue(),
		"Namespace %q should be controlled by %T %q", name, owningObj, owningObj.GetName())
}

func assertRoleBinding(ctx context.Context, namespace, name, owner string, owningObj client.Object) {
	GinkgoHelper()

	rb := &rbacv1.RoleBinding{}
	objectKey := client.ObjectKey{Namespace: namespace, Name: name}
	Expect(k8sClient.Get(ctx, objectKey, rb)).To(Succeed())

	Expect(rb.RoleRef).To(Equal(rbacv1.RoleRef{
		APIGroup: rbacv1.GroupName,
		Kind:     "ClusterRole",
		Name:     "admin",
	}), "RoleBinding %s/%s should reference the admin ClusterRole", namespace, name)

	Expect(rb.Subjects).To(ConsistOf(rbacv1.Subject{
		APIGroup: rbacv1.GroupName,
		Kind:     rbacv1.UserKind,
		Name:     owner,
	}), "RoleBinding %s/%s should grant access to user %q", namespace, name, owner)

	Expect(metav1.IsControlledBy(rb, owningObj)).To(BeTrue(),
		"RoleBinding %s/%s should be controlled by %T %q", namespace, name, owningObj, owningObj.GetName())
}
