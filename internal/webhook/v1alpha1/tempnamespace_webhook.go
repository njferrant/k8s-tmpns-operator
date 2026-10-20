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
	"context"
	"strconv"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tmpnsv1alpha1 "github.com/njferrant/k8s-tmpns-operator/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// nolint:unused
// log is for logging in this package.
var tempnamespacelog = logf.Log.WithName("tempnamespace-resource")

// SetupTempNamespaceWebhookWithManager registers the webhook for TempNamespace in the manager.
func SetupTempNamespaceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &tmpnsv1alpha1.TempNamespace{}).
		WithDefaulter(&TempNamespaceDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-tmpns-io-v1alpha1-tempnamespace,mutating=true,failurePolicy=fail,sideEffects=None,groups=tmpns.io,resources=tempnamespaces,verbs=create;update,versions=v1alpha1,name=mtempnamespace-v1alpha1.kb.io,admissionReviewVersions=v1

// TempNamespaceDefaulter struct is responsible for setting default values on the custom resource of the
// Kind TempNamespace when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type TempNamespaceDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

// Default implements admission.Defaulter so a webhook will be registered for the Kind TempNamespace.
func (d *TempNamespaceDefaulter) Default(ctx context.Context, obj *tmpnsv1alpha1.TempNamespace) error {
	tempnamespacelog.Info("Defaulting for TempNamespace", "name", obj.GetName())
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		tempnamespacelog.Error(err, "no admission request in context", "name", obj.GetName())
		return err
	}
	switch req.Operation {
	case admissionv1.Create:
		tempnamespacelog.Info("Recieved 'CREATE' request for", "name", obj.GetName())
		tempnamespacelog.Info("Setting", "name", obj.GetName(), "owner", req.UserInfo.Username)
		// Set owner to be requesting user
		obj.Spec.Owner = &tmpnsv1alpha1.ResourceOwner{
			Username: req.UserInfo.Username,
			UID:      req.UserInfo.UID,
		}

		// Set createdAt time
		now := metav1.Now()
		tempnamespacelog.Info("Setting", "name", obj.GetName(), "createdAt", now)
		obj.Spec.CreatedAt = &now

		// If a duration string is provided, attempt to parse it and calculate the expiration timestamp.
		// If it's missing or too short, pass through silently to let the VAP enforce field constraints.
		if obj.Spec.Duration != "" && len(obj.Spec.Duration) > 1 {
			d := obj.Spec.Duration
			valStr, unit := d[:len(d)-1], d[len(d)-1:]

			val, err := strconv.Atoi(valStr)
			if err != nil {
				// Return nil to allow the request to reach the VAP phase.
				// Returning a hard error here would bypass the VAP validation layer.
				return nil
			}

			var expiresAt time.Time
			switch unit {
			case "h":
				expiresAt = now.Add(time.Duration(val) * time.Hour)
			case "d":
				expiresAt = now.AddDate(0, 0, val)
			case "w":
				expiresAt = now.AddDate(0, 0, (7 * val))
			case "m":
				expiresAt = now.AddDate(0, val, 0)
			}

			// If no supported time unit matched, expiresAt remains its zero-value.
			// Leave the object untouched so the VAP can evaluate and reject the invalid format.
			if !expiresAt.IsZero() {
				tempnamespacelog.Info("Setting", "name", obj.GetName(), "expiresAt", expiresAt)
				obj.Spec.ExpiresAt = &metav1.Time{
					Time: expiresAt,
				}
				// Clear the duration string since the field is now redundant after mutation.
				tempnamespacelog.Info("Removing duration field (no longer needed) for", "name", obj.GetName())
				obj.Spec.Duration = ""
			} else {
				tempnamespacelog.Info("Could not determine 'expiredAt' for", "name", obj.GetName(), "duration", d)
			}
		}
	case admissionv1.Update:
		tempnamespacelog.Info("Recieved 'UPDATE' request for", "name", obj.GetName())
		// Clear duration string on UPDATE as expiresAt should be modified instead.
		// TODO: Figure out how to return a warning to the user if this path is hit.
		if obj.Spec.Duration != "" {
			tempnamespacelog.Info("Removing duration field (no longer needed) for", "name", obj.GetName())
			obj.Spec.Duration = ""
		}
	}
	return nil
}
