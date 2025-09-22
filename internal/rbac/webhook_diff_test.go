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

package rbac

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

var _ = Describe("WebhookDiffAnalyzer", func() {
	var (
		builder *RoleBindingBuilder
		scheme  *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		// Register the FolderTree type in the scheme
		err := rbacv1alpha1.AddToScheme(scheme)
		Expect(err).NotTo(HaveOccurred())

		builder = &RoleBindingBuilder{
			Scheme: scheme,
		}
	})

	Context("FolderTree to FolderTree comparison", func() {
		It("should detect CREATE operations for new FolderTree", func() {
			// Old state: nil (create operation)
			// New state: FolderTree with some folders
			newFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "test-folder",
							Namespaces: []string{"test-ns"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "test-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
					},
				},
			}

			builder.FolderTree = newFolderTree
			analyzer := NewWebhookDiffAnalyzer(nil, newFolderTree, builder)

			operations, err := analyzer.AnalyzeFolderTreeDiff()
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(1))

			op := operations[0]
			Expect(op.Type).To(Equal(OperationCreate))
			Expect(op.Namespace).To(Equal("test-ns"))
			Expect(op.RoleBindingTemplate.Name).To(Equal("test-template"))
		})

		It("should detect UPDATE operations when templates change", func() {
			// Old state: FolderTree with original template
			oldFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "test-folder",
							Namespaces: []string{"test-ns"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "old-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
					},
				},
			}

			// New state: Same structure but different subjects
			newFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "test-folder",
							Namespaces: []string{"test-ns"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "new-group", // Changed subject
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
					},
				},
			}

			builder.FolderTree = newFolderTree
			analyzer := NewWebhookDiffAnalyzer(oldFolderTree, newFolderTree, builder)

			operations, err := analyzer.AnalyzeFolderTreeDiff()
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(1))

			op := operations[0]
			Expect(op.Type).To(Equal(OperationUpdate))
			Expect(op.Namespace).To(Equal("test-ns"))
			Expect(op.RoleBindingTemplate.Name).To(Equal("test-template"))
		})

		It("should detect DELETE operations when folders are removed", func() {
			// Old state: FolderTree with folder
			oldFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "test-folder",
							Namespaces: []string{"test-ns"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "test-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
					},
				},
			}

			// New state: Empty FolderTree
			newFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{}, // No folders
				},
			}

			builder.FolderTree = newFolderTree
			analyzer := NewWebhookDiffAnalyzer(oldFolderTree, newFolderTree, builder)

			operations, err := analyzer.AnalyzeFolderTreeDiff()
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(1))

			op := operations[0]
			Expect(op.Type).To(Equal(OperationDelete))
			Expect(op.Namespace).To(Equal("test-ns"))
			Expect(op.RoleBindingTemplate.Name).To(Equal("test-template"))
		})

		It("should handle propagate field changes correctly", func() {
			boolPtr := func(b bool) *bool { return &b }

			// Old state: Template without propagate (defaults to false)
			oldFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name: "parent",
						Subfolders: []rbacv1alpha1.TreeNode{
							{Name: "child"},
						},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "parent",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "parent-template",
									// No Propagate field - defaults to false
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "parent-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
						{
							Name:       "child",
							Namespaces: []string{"child-ns"},
						},
					},
				},
			}

			// New state: Same template but with propagate: true
			newFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name: "parent",
						Subfolders: []rbacv1alpha1.TreeNode{
							{Name: "child"},
						},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "parent",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name:      "parent-template",
									Propagate: boolPtr(true), // Now propagates
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "parent-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
						{
							Name:       "child",
							Namespaces: []string{"child-ns"},
						},
					},
				},
			}

			builder.FolderTree = newFolderTree
			analyzer := NewWebhookDiffAnalyzer(oldFolderTree, newFolderTree, builder)

			operations, err := analyzer.AnalyzeFolderTreeDiff()
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(1)) // Should create 1 new RoleBinding in child-ns

			op := operations[0]
			Expect(op.Type).To(Equal(OperationCreate))
			Expect(op.Namespace).To(Equal("child-ns"))
			Expect(op.RoleBindingTemplate.Name).To(Equal("parent-template"))
		})

		It("should handle complex inheritance changes", func() {
			boolPtr := func(b bool) *bool { return &b }

			// Old state: Simple structure
			oldFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name: "root",
						Subfolders: []rbacv1alpha1.TreeNode{
							{Name: "app"},
						},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "root",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name:      "root-admin",
									Propagate: boolPtr(true),
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "admins",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
								},
							},
						},
						{
							Name:       "app",
							Namespaces: []string{"app-ns"},
						},
					},
				},
			}

			// New state: Add new namespace and template
			newFolderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Tree: &rbacv1alpha1.TreeNode{
						Name: "root",
						Subfolders: []rbacv1alpha1.TreeNode{
							{Name: "app"},
						},
					},
					Folders: []rbacv1alpha1.Folder{
						{
							Name: "root",
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name:      "root-admin",
									Propagate: boolPtr(true),
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "admins",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "admin",
									},
								},
							},
						},
						{
							Name:       "app",
							Namespaces: []string{"app-ns", "app-stage"}, // Added namespace
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "app-developers", // Added template
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "developers",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "edit",
									},
								},
							},
						},
					},
				},
			}

			builder.FolderTree = newFolderTree
			analyzer := NewWebhookDiffAnalyzer(oldFolderTree, newFolderTree, builder)

			operations, err := analyzer.AnalyzeFolderTreeDiff()
			Expect(err).NotTo(HaveOccurred())

			// Should have 3 CREATE operations:
			// 1. root-admin in app-stage (new namespace)
			// 2. app-developers in app-ns (new template)
			// 3. app-developers in app-stage (new template + new namespace)
			Expect(operations).To(HaveLen(3))

			createOps := []RoleBindingOperation{}
			for _, op := range operations {
				Expect(op.Type).To(Equal(OperationCreate))
				createOps = append(createOps, op)
			}

			// Verify we have the expected operations
			templateNames := make(map[string]bool)
			namespaces := make(map[string]bool)
			for _, op := range createOps {
				templateNames[op.RoleBindingTemplate.Name] = true
				namespaces[op.Namespace] = true
			}

			Expect(templateNames).To(HaveKey("root-admin"))
			Expect(templateNames).To(HaveKey("app-developers"))
			Expect(namespaces).To(HaveKey("app-ns"))
			Expect(namespaces).To(HaveKey("app-stage"))
		})

		It("should handle no changes correctly", func() {
			// Identical old and new states
			folderTree := &rbacv1alpha1.FolderTree{
				ObjectMeta: metav1.ObjectMeta{Name: "test-tree"},
				Spec: rbacv1alpha1.FolderTreeSpec{
					Folders: []rbacv1alpha1.Folder{
						{
							Name:       "test-folder",
							Namespaces: []string{"test-ns"},
							RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
								{
									Name: "test-template",
									Subjects: []rbacv1.Subject{
										{
											Kind:     "Group",
											Name:     "test-group",
											APIGroup: "rbac.authorization.k8s.io",
										},
									},
									RoleRef: rbacv1.RoleRef{
										APIGroup: "rbac.authorization.k8s.io",
										Kind:     "ClusterRole",
										Name:     "view",
									},
								},
							},
						},
					},
				},
			}

			builder.FolderTree = folderTree
			analyzer := NewWebhookDiffAnalyzer(folderTree, folderTree, builder)

			operations, err := analyzer.AnalyzeFolderTreeDiff()
			Expect(err).NotTo(HaveOccurred())
			Expect(operations).To(HaveLen(0)) // No changes = no operations
		})
	})
})
