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

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/folders/test/utils"
)

// Helper functions for common e2e test patterns

// createLimitedUserRBAC creates RBAC for a user with only FolderTree permissions (no RoleBinding permissions)
func createLimitedUserRBAC(userName, clusterRoleName, clusterRoleBindingName string) error {
	rbacYAML := fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
- apiGroups: ["rbac.kubevirt.io"]
  resources: ["foldertrees"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
# NOTE: Deliberately NOT granting rolebindings permissions to test privilege escalation
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s
subjects:
- kind: User
  name: %s
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: %s
  apiGroup: rbac.authorization.k8s.io
`, clusterRoleName, clusterRoleBindingName, userName, clusterRoleName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(rbacYAML)
	_, err := utils.Run(cmd)
	return err
}

// createSufficientUserRBAC creates RBAC for a user with both FolderTree and RoleBinding permissions, plus view permissions
func createSufficientUserRBAC(userName, clusterRoleName, clusterRoleBindingName string) error {
	rbacYAML := fmt.Sprintf(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
- apiGroups: ["rbac.kubevirt.io"]
  resources: ["foldertrees"]
  verbs: ["get", "list", "create", "update", "patch", "delete"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["rolebindings"]
  verbs: ["get", "list", "create", "update", "delete"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s-foldertree
subjects:
- kind: User
  name: %s
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: %s
  apiGroup: rbac.authorization.k8s.io
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: %s-view
subjects:
- kind: User
  name: %s
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: ClusterRole
  name: view  # Grant actual view ClusterRole permissions
  apiGroup: rbac.authorization.k8s.io
`, clusterRoleName, clusterRoleBindingName, userName, clusterRoleName, clusterRoleBindingName, userName)

	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(rbacYAML)
	_, err := utils.Run(cmd)
	return err
}

// cleanupUserRBAC cleans up RBAC resources for a test user
func cleanupUserRBAC(clusterRoleName, clusterRoleBindingName string, hasViewBinding bool) {
	utils.Run(exec.Command("kubectl", "delete", "clusterrole", clusterRoleName, "--ignore-not-found"))
	utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", clusterRoleBindingName, "--ignore-not-found"))
	if hasViewBinding {
		utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", fmt.Sprintf("%s-foldertree", clusterRoleBindingName), "--ignore-not-found"))
		utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", fmt.Sprintf("%s-view", clusterRoleBindingName), "--ignore-not-found"))
	}
}

// expectPrivilegeEscalationError expects a command to fail with privilege escalation error
func expectPrivilegeEscalationError(cmd *exec.Cmd, message string) {
	_, err := utils.Run(cmd)
	Expect(err).To(HaveOccurred(), message)
	Expect(err.Error()).To(Or(
		ContainSubstring("privilege escalation prevented"),
		ContainSubstring("dry-run creation failed"),
		ContainSubstring("forbidden"),
	))
}

// namespace where the project is deployed in
const namespace = "foldertree-system"

// serviceAccountName created for the project
const serviceAccountName = "foldertree-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "foldertree-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "foldertree-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("ensuring ClusterRoleBinding for metrics access exists")
			// Delete existing ClusterRoleBinding if it exists (cleanup from previous runs)
			deleteCmd := exec.Command("kubectl", "delete", "clusterrolebinding", metricsRoleBindingName, "--ignore-not-found")
			_, _ = utils.Run(deleteCmd) // Ignore errors - it's OK if it doesn't exist

			// Create the ClusterRoleBinding
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=foldertree-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for validating webhooks", func() {
			By("checking CA injection for validating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"validatingwebhookconfigurations.admissionregistration.k8s.io",
					"foldertree-validating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// FolderTree functionality tests - run while controller is still deployed
		Context("FolderTree Controller Functionality", func() {
			var testNamespaces []string

			BeforeAll(func() {
				// Create test namespaces for FolderTree functionality tests
				testNamespaces = []string{
					"ft-test-prod-web",
					"ft-test-prod-api",
					"ft-test-staging",
					"ft-test-sandbox",
				}

				By("granting cluster-admin permissions for FolderTree testing")
				// The webhook validates that users have the permissions they're trying to grant
				// For e2e testing, we need cluster-admin to create RoleBindings with any permissions
				cmd := exec.Command("kubectl", "create", "clusterrolebinding", "e2e-test-admin",
					"--clusterrole=cluster-admin",
					"--user=system:admin",
					"--group=system:masters")
				_, _ = utils.Run(cmd) // Ignore error if it already exists

				By("creating test namespaces")
				for _, ns := range testNamespaces {
					cmd := exec.Command("kubectl", "create", "namespace", ns, "--dry-run=client", "-o", "yaml")
					output, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())

					cmd = exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(output)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred())
				}

				// Wait a moment for namespaces to be ready
				time.Sleep(2 * time.Second)
			})

			AfterAll(func() {
				By("cleaning up test namespaces and FolderTrees")
				// Delete test FolderTrees
				utils.Run(exec.Command("kubectl", "delete", "foldertree", "--all", "--ignore-not-found"))

				// Delete test namespaces
				for _, ns := range testNamespaces {
					utils.Run(exec.Command("kubectl", "delete", "namespace", ns, "--ignore-not-found"))
				}

				// Clean up test ClusterRoleBinding
				utils.Run(exec.Command("kubectl", "delete", "clusterrolebinding", "e2e-test-admin", "--ignore-not-found"))
			})

			Context("Basic FolderTree Operations", func() {
				It("should validate a simple FolderTree first", func() {
					By("testing FolderTree validation with dry-run")
					simpleYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: validation-test
spec:
  folders:
  - name: simple-folder
    roleBindingTemplates:
    - name: simple-role
      subjects:
      - kind: Group
        name: test-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-sandbox"]
`

					cmd := exec.Command("kubectl", "apply", "--dry-run=server", "-f", "-")
					cmd.Stdin = strings.NewReader(simpleYAML)
					output, err := utils.Run(cmd)
					if err != nil {
						_, _ = fmt.Fprintf(GinkgoWriter, "Dry-run validation failed. Output: %s\n", output)
						_, _ = fmt.Fprintf(GinkgoWriter, "Error: %v\n", err)
					}
					Expect(err).NotTo(HaveOccurred(), "FolderTree should pass validation")
				})

				It("should create a simple FolderTree and generate RoleBindings", func() {
					By("creating a basic FolderTree")
					folderTreeYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: basic-test
spec:
  tree:
    name: platform
    subfolders:
    - name: production
      subfolders:
      - name: web-app
  folders:
  - name: platform
    roleBindingTemplates:
    - name: platform-admin
      propagate: true
      subjects:
      - kind: Group
        name: platform-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
  - name: production
    roleBindingTemplates:
    - name: prod-ops
      propagate: true
      subjects:
      - kind: Group
        name: prod-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-prod-api"]
  - name: web-app
    roleBindingTemplates:
    - name: web-developers
      subjects:
      - kind: Group
        name: web-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-prod-web"]
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(folderTreeYAML)
					output, err := utils.Run(cmd)
					if err != nil {
						_, _ = fmt.Fprintf(GinkgoWriter, "Failed to create FolderTree. Output: %s\n", output)
						_, _ = fmt.Fprintf(GinkgoWriter, "Error: %v\n", err)
						_, _ = fmt.Fprintf(GinkgoWriter, "YAML being applied:\n%s\n", folderTreeYAML)
					}
					Expect(err).NotTo(HaveOccurred(), "Failed to create FolderTree")

					By("verifying FolderTree was created")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "foldertree", "basic-test", "-o", "jsonpath={.metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(Equal("basic-test"))
					}).Should(Succeed())

					By("verifying RoleBindings were created with proper inheritance")
					// ft-test-prod-web should get: platform-admin + prod-ops + web-developers
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-prod-web", "-o", "jsonpath={.items[*].metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("platform-admin"))
						g.Expect(output).To(ContainSubstring("prod-ops"))
						g.Expect(output).To(ContainSubstring("web-developers"))
					}).Should(Succeed())

					// ft-test-prod-api should get: platform-admin + prod-ops (no web-developers)
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-prod-api", "-o", "jsonpath={.items[*].metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("platform-admin"))
						g.Expect(output).To(ContainSubstring("prod-ops"))
						g.Expect(output).NotTo(ContainSubstring("web-developers"))
					}).Should(Succeed())

					By("verifying RoleBinding content is correct")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebinding", "-n", "ft-test-prod-web", "-l", "foldertree.rbac.kubevirt.io/tree=basic-test", "-o", "yaml")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("platform-team"))
						g.Expect(output).To(ContainSubstring("prod-team"))
						g.Expect(output).To(ContainSubstring("web-team"))
						g.Expect(output).To(ContainSubstring("kind: ClusterRole"))
						g.Expect(output).To(ContainSubstring("name: view"))
					}).Should(Succeed())
				})

				AfterEach(func() {
					// Clean up FolderTrees after each test to prevent naming conflicts
					utils.Run(exec.Command("kubectl", "delete", "foldertree", "--all", "--ignore-not-found"))
					time.Sleep(1 * time.Second) // Wait for cleanup
				})

				It("should handle selective propagation correctly", func() {
					By("creating a FolderTree with selective propagation")
					folderTreeYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: propagation-test
spec:
  tree:
    name: prop-root
    subfolders:
    - name: prop-production
  folders:
  - name: prop-root
    roleBindingTemplates:
    - name: global-admin
      propagate: true
      subjects:
      - kind: Group
        name: global-admins
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    - name: secrets-access
      # No propagate field - defaults to false
      subjects:
      - kind: Group
        name: security-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
  - name: prop-production
    namespaces: ["ft-test-staging"]
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(folderTreeYAML)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to create FolderTree")

					By("verifying propagated permissions exist")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-staging", "-o", "jsonpath={.items[*].metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("global-admin"))
					}).Should(Succeed())

					By("verifying non-propagated permissions do NOT exist")
					Consistently(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-staging", "-o", "jsonpath={.items[*].metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).NotTo(ContainSubstring("secrets-access"))
					}).Should(Succeed())
				})

				It("should support standalone folders outside tree structure", func() {
					By("creating a FolderTree with standalone folders")
					folderTreeYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: standalone-test
spec:
  tree:
    name: main-org
  folders:
  - name: main-org
    roleBindingTemplates:
    - name: org-admin
      subjects:
      - kind: Group
        name: org-team
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
  - name: sandbox
    # Standalone folder - not part of tree
    roleBindingTemplates:
    - name: sandbox-users
      subjects:
      - kind: Group
        name: developers
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-sandbox"]
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(folderTreeYAML)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to create FolderTree")

					By("verifying standalone folder RoleBindings were created")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-sandbox", "-o", "jsonpath={.items[*].metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("sandbox-users"))
						g.Expect(output).NotTo(ContainSubstring("org-admin")) // Should not inherit from tree
					}).Should(Succeed())
				})
			})

			Context("Webhook Validation", func() {
				It("should reject invalid FolderTree configurations", func() {
					By("attempting to create FolderTree with duplicate folder names")
					invalidYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: invalid-test
spec:
  folders:
  - name: duplicate
    namespaces: ["ft-test-prod-web"]
  - name: duplicate
    namespaces: ["ft-test-prod-api"]
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(invalidYAML)
					_, err := utils.Run(cmd)
					Expect(err).To(HaveOccurred(), "Should have rejected duplicate folder names")
					Expect(err.Error()).To(ContainSubstring("already used"))
				})

				It("should reject FolderTree with namespace conflicts", func() {
					By("creating first FolderTree")
					firstYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: first-tree
spec:
  folders:
  - name: folder1
    namespaces: ["ft-test-prod-web"]
    roleBindingTemplates:
    - name: test-role
      subjects:
      - kind: Group
        name: test-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(firstYAML)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "First FolderTree should be created successfully")

					By("attempting to create second FolderTree with conflicting namespace")
					conflictYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: conflict-tree
spec:
  folders:
  - name: folder2
    namespaces: ["ft-test-prod-web"]  # Same namespace as first tree
    roleBindingTemplates:
    - name: test-role2
      subjects:
      - kind: Group
        name: test-group2
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
`

					cmd = exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(conflictYAML)
					_, err = utils.Run(cmd)
					Expect(err).To(HaveOccurred(), "Should have rejected conflicting namespace assignment")
					Expect(err.Error()).To(ContainSubstring("already assigned"))
				})

				It("should reject FolderTree with invalid DNS names", func() {
					By("attempting to create FolderTree with invalid folder name")
					invalidYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: invalid-names-test
spec:
  folders:
  - name: Invalid_Name_With_Underscores
    namespaces: ["ft-test-sandbox"]
    roleBindingTemplates:
    - name: test-role
      subjects:
      - kind: Group
        name: test-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(invalidYAML)
					_, err := utils.Run(cmd)
					Expect(err).To(HaveOccurred(), "Should have rejected invalid DNS name")
					Expect(err.Error()).To(ContainSubstring("DNS-1123"))
				})
			})

			Context("Security and Privilege Escalation Prevention", func() {
				var testSuffix string

				BeforeEach(func() {
					// Generate unique suffix for each test to avoid parallel execution conflicts
					testSuffix = fmt.Sprintf("%d", time.Now().UnixNano())
				})

				It("should prevent privilege escalation - user lacks permissions they're trying to grant", func() {
					userName := fmt.Sprintf("limited-test-user-%s", testSuffix)
					clusterRoleName := fmt.Sprintf("e2e-limited-permissions-%s", testSuffix)
					clusterRoleBindingName := fmt.Sprintf("e2e-limited-binding-%s", testSuffix)
					folderTreeName := fmt.Sprintf("escalation-test-%s", testSuffix)

					By("setting up RBAC for limited user")
					err := createLimitedUserRBAC(userName, clusterRoleName, clusterRoleBindingName)
					Expect(err).NotTo(HaveOccurred(), "Failed to create limited user RBAC")

					By("attempting to create FolderTree when user lacks RoleBinding create permissions")
					escalationYAML := fmt.Sprintf(`
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: %s
spec:
  folders:
  - name: escalation-folder
    roleBindingTemplates:
    - name: admin-access
      subjects:
      - kind: User
        name: %s
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin  # User doesn't have admin permissions!
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-staging"]
`, folderTreeName, userName)

					// Use kubectl --as to impersonate the limited user
					cmd := exec.Command("kubectl", "apply", "-f", "-", fmt.Sprintf("--as=%s", userName), "--as-group=system:authenticated")
					cmd.Stdin = strings.NewReader(escalationYAML)
					expectPrivilegeEscalationError(cmd, "Should have prevented privilege escalation")

					By("cleaning up test resources")
					utils.Run(exec.Command("kubectl", "delete", "foldertree", folderTreeName, "--ignore-not-found"))
					cleanupUserRBAC(clusterRoleName, clusterRoleBindingName, false)
				})

				It("should prevent privilege escalation - user lacks RoleBinding management permissions", func() {
					userName := fmt.Sprintf("no-rolebinding-user-%s", testSuffix)
					clusterRoleName := fmt.Sprintf("e2e-no-rolebinding-permissions-%s", testSuffix)
					clusterRoleBindingName := fmt.Sprintf("e2e-no-rolebinding-binding-%s", testSuffix)
					folderTreeName := fmt.Sprintf("no-rolebinding-test-%s", testSuffix)

					By("setting up RBAC for user with FolderTree permissions but no RoleBinding permissions")
					err := createLimitedUserRBAC(userName, clusterRoleName, clusterRoleBindingName)
					Expect(err).NotTo(HaveOccurred(), "Failed to create no-rolebinding user")

					By("attempting to create FolderTree without RoleBinding permissions")
					noRoleBindingYAML := fmt.Sprintf(`
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: %s
spec:
  folders:
  - name: test-folder
    roleBindingTemplates:
    - name: view-access
      subjects:
      - kind: User
        name: %s
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-sandbox"]
`, folderTreeName, userName)

					cmd := exec.Command("kubectl", "apply", "-f", "-", fmt.Sprintf("--as=%s", userName), "--as-group=system:authenticated")
					cmd.Stdin = strings.NewReader(noRoleBindingYAML)
					expectPrivilegeEscalationError(cmd, "Should have prevented creation due to lack of RoleBinding permissions")

					By("cleaning up test resources")
					utils.Run(exec.Command("kubectl", "delete", "foldertree", folderTreeName, "--ignore-not-found"))
					cleanupUserRBAC(clusterRoleName, clusterRoleBindingName, false)
				})

				It("should allow FolderTree creation when user has sufficient permissions", func() {
					userName := fmt.Sprintf("sufficient-user-%s", testSuffix)
					clusterRoleName := fmt.Sprintf("e2e-sufficient-permissions-%s", testSuffix)
					clusterRoleBindingName := fmt.Sprintf("e2e-sufficient-binding-%s", testSuffix)
					folderTreeName := fmt.Sprintf("sufficient-test-%s", testSuffix)

					By("setting up RBAC for user with sufficient permissions")
					err := createSufficientUserRBAC(userName, clusterRoleName, clusterRoleBindingName)
					Expect(err).NotTo(HaveOccurred(), "Failed to create sufficient user")

					By("creating FolderTree with permissions user actually has")
					validYAML := fmt.Sprintf(`
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: %s
spec:
  folders:
  - name: sufficient-folder
    roleBindingTemplates:
    - name: view-access
      subjects:
      - kind: User
        name: %s
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view  # User has equivalent permissions
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-sandbox"]
`, folderTreeName, userName)

					cmd := exec.Command("kubectl", "apply", "-f", "-", fmt.Sprintf("--as=%s", userName), "--as-group=system:authenticated")
					cmd.Stdin = strings.NewReader(validYAML)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Should allow FolderTree creation with sufficient permissions")

					By("verifying FolderTree was created successfully")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "foldertree", folderTreeName, "-o", "jsonpath={.metadata.name}")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(Equal(folderTreeName))
					}).Should(Succeed())

					By("cleaning up test resources")
					utils.Run(exec.Command("kubectl", "delete", "foldertree", folderTreeName, "--ignore-not-found"))
					cleanupUserRBAC(clusterRoleName, clusterRoleBindingName, true)
				})

				It("should prevent updates when user lacks permissions for new templates", func() {
					userName := fmt.Sprintf("update-test-user-%s", testSuffix)
					clusterRoleName := fmt.Sprintf("e2e-update-limited-permissions-%s", testSuffix)
					clusterRoleBindingName := fmt.Sprintf("e2e-update-limited-binding-%s", testSuffix)
					folderTreeName := fmt.Sprintf("update-security-test-%s", testSuffix)

					By("setting up RBAC for limited user")
					err := createLimitedUserRBAC(userName, clusterRoleName, clusterRoleBindingName)
					Expect(err).NotTo(HaveOccurred(), "Failed to create limited user RBAC")

					By("creating initial FolderTree with cluster-admin")
					initialYAML := fmt.Sprintf(`
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: %s
spec:
  folders:
  - name: update-folder
    roleBindingTemplates:
    - name: basic-view
      subjects:
      - kind: User
        name: %s
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-staging"]
`, folderTreeName, userName)

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(initialYAML)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to create initial FolderTree")

					By("attempting to update FolderTree with permissions limited user doesn't have")
					updateYAML := fmt.Sprintf(`
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: %s
spec:
  folders:
  - name: update-folder
    roleBindingTemplates:
    - name: basic-view
      subjects:
      - kind: User
        name: %s
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    - name: admin-access
      subjects:
      - kind: User
        name: %s
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin  # Limited user doesn't have admin permissions!
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-staging"]
`, folderTreeName, userName, userName)

					cmd = exec.Command("kubectl", "apply", "-f", "-", fmt.Sprintf("--as=%s", userName), "--as-group=system:authenticated")
					cmd.Stdin = strings.NewReader(updateYAML)
					expectPrivilegeEscalationError(cmd, "Should have prevented update due to privilege escalation")

					By("cleaning up test resources")
					utils.Run(exec.Command("kubectl", "delete", "foldertree", folderTreeName, "--ignore-not-found"))
					cleanupUserRBAC(clusterRoleName, clusterRoleBindingName, false)
				})

				It("should prevent deletion when user lacks permissions to delete existing RoleBindings", func() {
					userName := fmt.Sprintf("delete-test-user-%s", testSuffix)
					clusterRoleName := fmt.Sprintf("e2e-delete-limited-permissions-%s", testSuffix)
					clusterRoleBindingName := fmt.Sprintf("e2e-delete-limited-binding-%s", testSuffix)
					folderTreeName := fmt.Sprintf("delete-security-test-%s", testSuffix)

					By("setting up RBAC for limited user")
					err := createLimitedUserRBAC(userName, clusterRoleName, clusterRoleBindingName)
					Expect(err).NotTo(HaveOccurred(), "Failed to create limited user RBAC")

					By("creating FolderTree with admin permissions (as cluster-admin)")
					adminFolderTreeYAML := fmt.Sprintf(`
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: %s
spec:
  folders:
  - name: admin-folder
    roleBindingTemplates:
    - name: admin-access
      subjects:
      - kind: Group
        name: admin-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: admin
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-staging"]
`, folderTreeName)

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(adminFolderTreeYAML)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to create admin FolderTree")

					By("verifying admin RoleBinding was created")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-staging", "-l", fmt.Sprintf("foldertree.rbac.kubevirt.io/tree=%s", folderTreeName))
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("admin-access"))
					}).Should(Succeed())

					By("attempting to delete FolderTree as limited user")
					// Limited user doesn't have permission to delete RoleBindings with admin permissions
					cmd = exec.Command("kubectl", "delete", "foldertree", folderTreeName, fmt.Sprintf("--as=%s", userName), "--as-group=system:authenticated")
					expectPrivilegeEscalationError(cmd, "Should have prevented deletion due to insufficient permissions")

					By("cleaning up test resources")
					utils.Run(exec.Command("kubectl", "delete", "foldertree", folderTreeName, "--ignore-not-found"))
					cleanupUserRBAC(clusterRoleName, clusterRoleBindingName, false)
				})
			})

			Context("Dynamic Updates", func() {
				It("should handle FolderTree updates and RoleBinding changes", func() {
					By("creating initial FolderTree")
					initialYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: update-test
spec:
  folders:
  - name: test-folder
    roleBindingTemplates:
    - name: initial-role
      subjects:
      - kind: Group
        name: initial-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-staging"]
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(initialYAML)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to create initial FolderTree")

					By("verifying initial RoleBinding exists")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-staging", "-o", "yaml")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("initial-role"))
						g.Expect(output).To(ContainSubstring("initial-group"))
					}).Should(Succeed())

					By("updating FolderTree with additional role")
					updatedYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: update-test
spec:
  folders:
  - name: test-folder
    roleBindingTemplates:
    - name: initial-role
      subjects:
      - kind: Group
        name: initial-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    - name: additional-role
      subjects:
      - kind: Group
        name: additional-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-staging"]
`

					cmd = exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(updatedYAML)
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to update FolderTree")

					By("verifying both RoleBindings exist after update")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-staging", "-o", "yaml")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("initial-role"))
						g.Expect(output).To(ContainSubstring("additional-role"))
						g.Expect(output).To(ContainSubstring("initial-group"))
						g.Expect(output).To(ContainSubstring("additional-group"))
					}).Should(Succeed())
				})

				It("should clean up RoleBindings when FolderTree is deleted", func() {
					By("creating FolderTree for deletion test")
					deleteTestYAML := `
apiVersion: rbac.kubevirt.io/v1alpha1
kind: FolderTree
metadata:
  name: delete-test
spec:
  folders:
  - name: temp-folder
    roleBindingTemplates:
    - name: temp-role
      subjects:
      - kind: Group
        name: temp-group
        apiGroup: rbac.authorization.k8s.io
      roleRef:
        kind: ClusterRole
        name: view
        apiGroup: rbac.authorization.k8s.io
    namespaces: ["ft-test-sandbox"]
`

					cmd := exec.Command("kubectl", "apply", "-f", "-")
					cmd.Stdin = strings.NewReader(deleteTestYAML)
					_, err := utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to create FolderTree for deletion test")

					By("verifying RoleBinding was created")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-sandbox", "-l", "foldertree.rbac.kubevirt.io/tree=delete-test")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).To(ContainSubstring("temp-role"))
					}).Should(Succeed())

					By("deleting the FolderTree")
					cmd = exec.Command("kubectl", "delete", "foldertree", "delete-test")
					_, err = utils.Run(cmd)
					Expect(err).NotTo(HaveOccurred(), "Failed to delete FolderTree")

					By("verifying RoleBindings were cleaned up")
					Eventually(func(g Gomega) {
						cmd := exec.Command("kubectl", "get", "rolebindings", "-n", "ft-test-sandbox", "-l", "foldertree.rbac.kubevirt.io/tree=delete-test")
						output, err := utils.Run(cmd)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(output).NotTo(ContainSubstring("temp-role"))
					}).Should(Succeed())
				})
			})
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
