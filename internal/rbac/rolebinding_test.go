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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	rbacv1alpha1 "kubevirt.io/folders/api/v1alpha1"
)

func TestRBAC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RBAC Package Suite")
}

var _ = Describe("RoleBindingBuilder", func() {
	var (
		folderTree *rbacv1alpha1.FolderTree
		builder    *RoleBindingBuilder
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		rbacv1alpha1.AddToScheme(scheme)
		rbacv1.AddToScheme(scheme)

		folderTree = &rbacv1alpha1.FolderTree{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-tree",
			},
			Spec: rbacv1alpha1.FolderTreeSpec{
				Folders: []rbacv1alpha1.Folder{
					{
						Name: "test-folder",
						RoleBindingTemplates: []rbacv1alpha1.RoleBindingTemplate{
							{
								Name: "test-permission",
								Subjects: []rbacv1.Subject{
									{
										Kind:     "User",
										Name:     "test-user",
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
				},
			},
		}
	})

	Context("BuildRoleBindingFromTemplate", func() {
		var testRoleBindingTemplate rbacv1alpha1.RoleBindingTemplate

		BeforeEach(func() {
			testRoleBindingTemplate = rbacv1alpha1.RoleBindingTemplate{
				Name: "test-permission",
				Subjects: []rbacv1.Subject{
					{
						Kind:     "User",
						Name:     "test-user",
						APIGroup: "rbac.authorization.k8s.io",
					},
				},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     "admin",
				},
			}
		})

		It("should create RoleBinding with owner reference when scheme provided", func() {
			builder = &RoleBindingBuilder{
				FolderTree: folderTree,
				Scheme:     scheme,
			}

			roleBinding, err := builder.BuildRoleBindingFromTemplate("test-namespace", testRoleBindingTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding).NotTo(BeNil())

			// Verify basic properties
			Expect(roleBinding.Name).To(Equal("foldertree-test-tree-test-permission"))
			Expect(roleBinding.Namespace).To(Equal("test-namespace"))
			Expect(roleBinding.Subjects).To(HaveLen(1))
			Expect(roleBinding.Subjects[0].Name).To(Equal("test-user"))
			Expect(roleBinding.RoleRef.Name).To(Equal("admin"))

			// Verify labels
			Expect(roleBinding.Labels["app.kubernetes.io/managed-by"]).To(Equal("foldertree-controller"))
			Expect(roleBinding.Labels["foldertree.rbac.kubevirt.io/tree"]).To(Equal("test-tree"))
			Expect(roleBinding.Labels["foldertree.rbac.kubevirt.io/role-binding-template"]).To(Equal("test-permission"))

			// Verify owner reference is set
			Expect(roleBinding.OwnerReferences).To(HaveLen(1))
			Expect(roleBinding.OwnerReferences[0].Name).To(Equal("test-tree"))
		})

		It("should create RoleBinding without owner reference when scheme not provided", func() {
			builder = &RoleBindingBuilder{
				FolderTree: folderTree,
				Scheme:     nil, // No scheme - for webhook usage
			}

			roleBinding, err := builder.BuildRoleBindingFromTemplate("test-namespace", testRoleBindingTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(roleBinding).NotTo(BeNil())

			// Verify basic properties
			Expect(roleBinding.Name).To(Equal("foldertree-test-tree-test-permission"))
			Expect(roleBinding.Namespace).To(Equal("test-namespace"))

			// Verify no owner reference is set (for webhook dry-run)
			Expect(roleBinding.OwnerReferences).To(BeEmpty())
		})
	})

	Context("GenerateRandomRoleBindingName", func() {
		It("should generate names with expected format", func() {
			name := GenerateRandomRoleBindingName("tree1", "perm1")

			// Should contain expected components
			Expect(name).To(ContainSubstring("dryrun-foldertree-tree1-perm1"))
			Expect(name).To(MatchRegexp(`dryrun-foldertree-tree1-perm1-\d+`))
		})
	})
})
