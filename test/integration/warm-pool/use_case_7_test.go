package warm_pool

import (
	"fmt"
	"github.com/aws/amazon-vpc-cni-k8s/test/framework/resources/k8s/manifest"
	"github.com/aws/amazon-vpc-cni-k8s/test/framework/utils"
	"math/rand"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("use case 7", func() {
	Context("Single Burst Behavior", func() {

		BeforeEach(func() {
			By("Getting Warm Pool Environment Variables Before Test")
			getWarmPoolEnvVars()
		})

		It("Scales the cluster and checks warm pool before and after", func() {
			replicas := minPods

			fmt.Fprintf(GinkgoWriter, "Deploying %v minimum pods\n", minPods)
			deploymentSpec := manifest.NewBusyBoxDeploymentBuilder(f.Options.TestImageRegistry).
				Namespace("default").
				Name("busybox").
				NodeName(primaryNode.Name).
				Namespace(utils.DefaultTestNamespace).
				Replicas(replicas).
				Build()

			_, err := f.K8sResourceManagers.
				DeploymentManager().
				CreateAndWaitTillDeploymentIsReady(deploymentSpec, utils.DefaultDeploymentReadyTimeout*5)
			Expect(err).ToNot(HaveOccurred())

			if minPods != 0 {
				time.Sleep(sleep)
			}

			randBurst := rand.Intn(iterations)

			for i := 0; i < iterations; i++ {
				By("Loop " + strconv.Itoa(i))

				if i == randBurst {
					fmt.Fprintf(GinkgoWriter, "Burst behavior from %v to %v pods\n", replicas, maxPods)
					quickScale(maxPods)
					continue
				}

				if i == randBurst+1 {
					fmt.Fprintf(GinkgoWriter, "Burst behavior over, scaling down from %v to %v pods\n", maxPods,
						replicas)
					quickScale(replicas)
					continue
				}

				result, op := randOp(replicas, iterPods)
				replicas = checkInRange(result)
				fmt.Fprintf(GinkgoWriter, "%v %v pod to cluster to equal %v pods\n", op, iterPods, replicas)
				quickScale(replicas)
				Expect(replicas).To(Equal(busyboxPodCnt()))
			}

			Expect(minPods).To(Equal(busyboxPodCnt()))

			By("Deleting the deployment")
			err = f.K8sResourceManagers.DeploymentManager().DeleteAndWaitTillDeploymentIsDeleted(deploymentSpec)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			By("Getting Warm Pool Environment Variables After Test")
			getWarmPoolEnvVars()
		})
	})
})
