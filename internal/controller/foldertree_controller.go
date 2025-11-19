/*
Copyright 2025.

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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
	"kubevirt.io/folders/internal/rbac"
)

// FolderTreeReconciler reconciles a FolderTree object.
// The controller processes the split structure design where:
// - spec.tree defines hierarchical relationships between folders
// - spec.folders[] contains the actual inline role binding templates and namespace data
//
// The reconciler creates RoleBindings in namespaces based on:
// 1. Direct folder role binding templates for namespaces assigned to that folder
// 2. Inherited role binding templates from parent folders in the tree hierarchy
// 3. Standalone folders that exist outside any tree structure
type FolderTreeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=rbac.kubevirt.io,resources=foldertrees,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.kubevirt.io,resources=foldertrees/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *FolderTreeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the FolderTree instance
	folderTree := &rbacv1alpha1.FolderTree{}
	err := r.Get(ctx, req.NamespacedName, folderTree)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("FolderTree resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get FolderTree")
		return ctrl.Result{}, err
	}

	// No finalizers needed - RoleBindings have owner references and will be garbage collected automatically

	// Note: Validation is now handled by the validating webhook

	// Use diff analyzer to determine and execute only the required operations
	if err := r.processOperations(ctx, folderTree); err != nil {
		log.Error(err, "Failed to process RoleBinding operations")
		r.updateStatus(ctx, folderTree, rbacv1alpha1.ConditionTypeProcessingFailed, err.Error())
		return ctrl.Result{}, err // RequeueAfter is ignored when returning error - controller-runtime uses exponential backoff
	}

	// Update status
	r.updateStatus(ctx, folderTree, rbacv1alpha1.ConditionTypeReady, "FolderTree processed successfully")

	return ctrl.Result{}, nil // No requeue needed - watches handle all drift detection
}

// processOperations uses the diff analyzer to determine what operations are needed
// and executes only the required changes (create/update/delete)
func (r *FolderTreeReconciler) processOperations(ctx context.Context, folderTree *rbacv1alpha1.FolderTree) error {
	log := logf.FromContext(ctx)

	// Create diff analyzer to determine what operations are needed
	builder := &rbac.RoleBindingBuilder{
		FolderTree: folderTree,
		Scheme:     r.Scheme, // Include scheme for owner reference
	}

	diffAnalyzer := rbac.NewDiffAnalyzer(r.Client, folderTree, builder)

	// Analyze what operations are needed
	operations, err := diffAnalyzer.AnalyzeDiff(ctx)
	if err != nil {
		return fmt.Errorf("failed to analyze required operations: %v", err)
	}

	// Execute each operation
	for _, operation := range operations {
		if err := r.executeOperation(ctx, operation); err != nil {
			log.Error(err, "Failed to execute operation", "operation", operation.String())
			return err
		}
		log.Info("Successfully executed operation", "operation", operation.String())
	}

	return nil
}

// executeOperation executes a single RoleBinding operation (create/update/delete)
func (r *FolderTreeReconciler) executeOperation(ctx context.Context, operation rbac.RoleBindingOperation) error {
	switch operation.Type {
	case rbac.OperationCreate:
		return r.executeCreateOperation(ctx, operation)
	case rbac.OperationUpdate:
		return r.executeUpdateOperation(ctx, operation)
	case rbac.OperationDelete:
		return r.executeDeleteOperation(ctx, operation)
	default:
		return fmt.Errorf("unknown operation type: %s", operation.Type)
	}
}

// executeCreateOperation creates a new RoleBinding
func (r *FolderTreeReconciler) executeCreateOperation(ctx context.Context, operation rbac.RoleBindingOperation) error {
	log := logf.FromContext(ctx)

	// Check if namespace exists before creating RoleBinding
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: operation.Namespace}, ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Namespace not found, skipping RoleBinding creation", "namespace", operation.Namespace)
			return nil // Skip if namespace doesn't exist - will be applied when namespace is created
		}
		return err
	}

	log.Info("Creating RoleBinding", "name", operation.DesiredRoleBinding.Name, "namespace", operation.Namespace)
	return r.Create(ctx, operation.DesiredRoleBinding)
}

// executeUpdateOperation updates an existing RoleBinding
func (r *FolderTreeReconciler) executeUpdateOperation(ctx context.Context, operation rbac.RoleBindingOperation) error {
	log := logf.FromContext(ctx)

	// Update the existing RoleBinding with desired values
	existing := operation.ExistingRoleBinding
	existing.Subjects = operation.DesiredRoleBinding.Subjects
	existing.RoleRef = operation.DesiredRoleBinding.RoleRef
	existing.Labels = operation.DesiredRoleBinding.Labels

	log.Info("Updating RoleBinding", "name", existing.Name, "namespace", existing.Namespace)
	return r.Update(ctx, existing)
}

// executeDeleteOperation deletes an existing RoleBinding
func (r *FolderTreeReconciler) executeDeleteOperation(ctx context.Context, operation rbac.RoleBindingOperation) error {
	log := logf.FromContext(ctx)

	log.Info("Deleting RoleBinding", "name", operation.ExistingRoleBinding.Name, "namespace", operation.ExistingRoleBinding.Namespace)
	return r.Delete(ctx, operation.ExistingRoleBinding)
}

// updateStatus updates the status of the FolderTree
func (r *FolderTreeReconciler) updateStatus(ctx context.Context, folderTree *rbacv1alpha1.FolderTree, conditionType, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             conditionType,
		Message:            message,
	}

	// Clear conflicting conditions to ensure clean status
	switch conditionType {
	case rbacv1alpha1.ConditionTypeReady:
		// Remove ProcessingFailed when setting Ready
		r.removeCondition(folderTree, rbacv1alpha1.ConditionTypeProcessingFailed)
	case rbacv1alpha1.ConditionTypeProcessingFailed:
		// Remove Ready when setting ProcessingFailed
		r.removeCondition(folderTree, rbacv1alpha1.ConditionTypeReady)
	}

	// Update or add the condition
	updated := false
	for i, existing := range folderTree.Status.Conditions {
		if existing.Type == conditionType {
			folderTree.Status.Conditions[i] = condition
			updated = true
			break
		}
	}
	if !updated {
		folderTree.Status.Conditions = append(folderTree.Status.Conditions, condition)
	}

	folderTree.Status.ProcessedGeneration = folderTree.Generation

	// Update status - ignore error as status updates are best-effort
	_ = r.Status().Update(ctx, folderTree)
}

// removeCondition removes a condition by type
func (r *FolderTreeReconciler) removeCondition(folderTree *rbacv1alpha1.FolderTree, conditionType string) {
	for i, condition := range folderTree.Status.Conditions {
		if condition.Type == conditionType {
			folderTree.Status.Conditions = append(
				folderTree.Status.Conditions[:i],
				folderTree.Status.Conditions[i+1:]...,
			)
			break
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
// The controller uses an event-driven approach with comprehensive watches:
// - For(): Watches FolderTree resources for spec changes
// - Owns(): Watches RoleBinding resources for drift detection (delete/modify events)
// - Watches(): Watches Namespace resources for new namespace creation
// This eliminates the need for periodic requeuing since all relevant changes trigger reconciliation.
func (r *FolderTreeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rbacv1alpha1.FolderTree{}).
		Owns(&rbacv1.RoleBinding{}). // Handles drift: RoleBinding delete/modify triggers reconciliation
		Watches(&corev1.Namespace{}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
			// When a namespace is created/updated, reconcile all FolderTrees
			// to check if any need to create RoleBindings in the new namespace
			var requests []reconcile.Request
			folderTreeList := &rbacv1alpha1.FolderTreeList{}
			if err := mgr.GetClient().List(ctx, folderTreeList); err != nil {
				return requests
			}
			for _, ft := range folderTreeList.Items {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: ft.Name},
				})
			}
			return requests
		})).
		Named("foldertree").
		Complete(r)
}
