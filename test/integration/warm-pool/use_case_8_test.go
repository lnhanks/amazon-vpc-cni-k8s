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

var _ = Describe("use case 8", func() {
	Context("Multiple Burst Behavior", func() {

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

			// Creates some bursts of different sizes at random iterations.
			burstIdx := rand.Perm(iterations)[:numBursts]
			burstMap := make(map[int]int)
			for i := 0; i < len(burstIdx); i++ {
				key := burstIdx[i]
				//value := incIf(rand.Intn(maxPods + 1))
				value := int(maxPods / min(1, rand.Intn(4)))
				burstMap[key] = value
			}

			for i := 0; i < iterations; i++ {
				By("Loop " + strconv.Itoa(i))

				val, present := burstMap[i]
				if present {
					fmt.Fprintf(GinkgoWriter, "Burst behavior from %v to %v pods\n", replicas, val)
					quickScale(val)

					fmt.Fprintf(GinkgoWriter, "Burst behavior over, scaling down from %v to %v pods\n", val,
						replicas)
					quickScale(replicas)
					continue
				}

				result, op := randOp(replicas, iterPods)
				replicas = checkInRange(result)
				fmt.Fprintf(GinkgoWriter, "%v %v pod from cluster to equal %v pods\n", op, iterPods, replicas)
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
