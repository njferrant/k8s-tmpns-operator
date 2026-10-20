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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tmpnsv1alpha1 "github.com/njferrant/k8s-tmpns-operator/api/v1alpha1"
)

// TempNamespaceReconciler reconciles a TempNamespace object
type TempNamespaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=tmpns.io,resources=tempnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=tmpns.io,resources=tempnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=tmpns.io,resources=tempnamespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,resourceNames=admin,verbs=bind
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state. For more
// details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *TempNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var tn tmpnsv1alpha1.TempNamespace
	if err := r.Get(ctx, req.NamespacedName, &tn); err != nil {
		if errors.IsNotFound(err) {
			// TempNamespace not found. Since it doesn't exist, return and stop reconciliation
			log.Info("Object not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		// Error reading TempNamespace
		return ctrl.Result{}, err
	}
	// Check if TempNamespace is expired and handle deletion if so
	if isExpired(&tn, time.Now()) {
		return ctrl.Result{}, r.handleExpiry(ctx, &tn)
	}

	// Create the Namespace first; the RoleBinding below lives inside it.
	if err := r.handleNamespace(ctx, &tn); err != nil {
		return ctrl.Result{}, r.failWith(ctx, &tn, "NamespaceCreateOrUpdateFailed", err)
	}
	tn.Status.Namespace = tn.Name

	// Give tn's owner access to the new Namespace.
	if err := r.handleOwnerRbac(ctx, &tn); err != nil {
		return ctrl.Result{}, r.failWith(ctx, &tn, "RbacCreateOrUpdateFailed", err)
	}

	// All done! Set condition status to Ready=True.
	r.setCondition(
		&tn, "Ready", metav1.ConditionTrue, "Provisioned", "Namespace and owner RBAC rolebinding have finished provisioning",
	)
	if err := r.Status().Update(ctx, &tn); err != nil {
		return ctrl.Result{}, err
	}

	// Schedule the wakeup to handle expirey
	return ctrl.Result{RequeueAfter: time.Until(tn.Spec.ExpiresAt.Time)}, nil
}

// SetupWithManager registers the controller with mgr and defines which events
// trigger Reconcile:
//   - any create, update, or delete of a TempNamespace (the primary resource)
//   - any event on a Namespace or RoleBinding that has a controller reference
//     to a TempNamespace, which is mapped back to that TempNamespace
//
// Each event enqueues the key of the TempNamespace concerned, and Reconcile
// then compares the cluster against the desired state.
func (r *TempNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tmpnsv1alpha1.TempNamespace{}).
		Owns(&corev1.Namespace{}).
		Owns(&rbacv1.RoleBinding{}).
		Named("tempnamespace").
		Complete(r)
}

// setCondition sets the condition of the given type on tn's in-memory status,
// replacing any existing condition of that type, and records tn's current
// generation as the condition's ObservedGeneration. It does not write to the
// API server; the caller must persist the change with Status().Update.
func (r *TempNamespaceReconciler) setCondition(tn *tmpnsv1alpha1.TempNamespace, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: tn.Generation,
	}
	meta.SetStatusCondition(&tn.Status.Conditions, condition)
}

// handleExpiry deletes the expired TempNamespace tn. The Namespace and anything
// else controlled by tn are then removed by Kubernetes garbage collection. It
// returns nil if tn is already gone, so it is safe to call repeatedly.
func (r *TempNamespaceReconciler) handleExpiry(ctx context.Context, tn *tmpnsv1alpha1.TempNamespace) error {
	return client.IgnoreNotFound(r.Delete(ctx, tn))
}

// handleNamespace ensures the Namespace for tn exists with the managed label
// and a controller reference to tn, so deleting the TempNamespace garbage
// collects the Namespace. It returns a terminal error if a Namespace with the
// same name exists that is not controlled by tn.
func (r *TempNamespaceReconciler) handleNamespace(ctx context.Context, tn *tmpnsv1alpha1.TempNamespace) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: tn.Name},
	}
	// CreateOrUpdate does a Get on the Namespace, filling ns with the live object
	// if one exists, then runs the mutate function to set the desired state. It
	// calls Create or Update only after the mutate function returns without error,
	// and only if something changed.
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, ns, func() error {
		// A non-zero CreationTimestamp indicates a ns object was found (and already exists)
		if !ns.CreationTimestamp.IsZero() && !metav1.IsControlledBy(ns, tn) {
			return reconcile.TerminalError(fmt.Errorf("namespace %s exists and is not owned by this TempNamespace resource", ns.Name))
		}
		// Add a managed label
		if ns.ObjectMeta.Labels == nil {
			ns.ObjectMeta.Labels = map[string]string{}
		}
		ns.ObjectMeta.Labels["tmpns.io/managed"] = "true"
		// Set owner reference on the Namespace, linking it back to TempNamespace
		return controllerutil.SetControllerReference(tn, ns, r.Scheme)
	})
	if err != nil {
		return err
	}
	return nil
}

// handleOwnerRbac ensures tn's owner has the built-in admin role in tn's
// Namespace, through a RoleBinding controlled by tn. It returns a terminal
// error if a RoleBinding with the same name already exists that tn does not
// control, or that has a different roleRef, since neither can be fixed by
// retrying.
func (r *TempNamespaceReconciler) handleOwnerRbac(ctx context.Context, tn *tmpnsv1alpha1.TempNamespace) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-admin", tn.Name),
			Namespace: tn.Name,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		if !rb.CreationTimestamp.IsZero() {
			if !metav1.IsControlledBy(rb, tn) {
				return reconcile.TerminalError(fmt.Errorf("rolebinding %q exists and is not owned by %q", rb.Name, tn.Name))
			}
			// roleRef is immutable, so a mismatch can't be repaired by an
			// update. Failing here gives a clearer message than the API
			// server's immutability error.
			if rb.RoleRef.Name != "admin" {
				return reconcile.TerminalError(fmt.Errorf("rolebinding %q exists with unexpected RoleRef %q", rb.Name, rb.RoleRef.Name))
			}
		}
		rb.RoleRef = rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "admin",
			APIGroup: rbacv1.GroupName,
		}
		// Assigned on every run so that the owning user is authoritative: extra
		// subjects added in by someone else are removed on the next reconcile.
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:     rbacv1.UserKind,
				Name:     tn.Spec.Owner.Username,
				APIGroup: rbacv1.GroupName,
			},
		}
		// Set owner reference on the RoleBinding, linking it back to TempNamespace
		return controllerutil.SetControllerReference(tn, rb, r.Scheme)
	})
	if err != nil {
		return err
	}
	return nil
}

// failWith records a failure on tn by setting its Ready condition to False with
// the given reason and the text of err, then writes the status. A failure to
// write the status is logged rather than returned. It returns err unchanged so
// the caller can return it to controller-runtime, which requeues with backoff
// (or not, if err is wrapped with [reconcile.TerminalError]).
func (r *TempNamespaceReconciler) failWith(ctx context.Context, tn *tmpnsv1alpha1.TempNamespace, reason string, err error) error {
	r.setCondition(
		tn, "Ready", metav1.ConditionFalse, reason, err.Error(),
	)
	if statusErr := r.Status().Update(ctx, tn); err != nil {
		logf.FromContext(ctx).Error(statusErr, "failed to update status condition")
	}
	// original error, so controller-runtime requeues with backoff
	return err
}

// isExpired reports whether tn has reached its expiry time, meaning now is at or
// after tn.Spec.ExpiresAt.
func isExpired(tn *tmpnsv1alpha1.TempNamespace, now time.Time) bool {
	return !now.Before(tn.Spec.ExpiresAt.Time)
}
