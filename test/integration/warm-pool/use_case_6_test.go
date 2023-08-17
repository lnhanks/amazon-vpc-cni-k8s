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

var _ = Describe("use case 6", func() {
	Context("Random Scaling", func() {

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

			for replicas < maxPods {
				i := 0
				randScale := rand.Intn(randDigits) + 1 // prevent divide by 0
				scaleAmt := int(maxPods / randScale)
				By("Loop " + strconv.Itoa(i))
				// Will scale to a maximum of maxPods
				replicas = min(replicas+scaleAmt, maxPods)
				fmt.Fprintf(GinkgoWriter, "Scaling cluster up to %v pods by adding %v pods\n", replicas, scaleAmt)
				quickScale(replicas)
				Expect(replicas).To(Equal(busyboxPodCnt()))
				i++
			}

			for replicas > minPods {
				i := 0
				randScale := rand.Intn(randDigits)
				scaleAmt := int(maxPods/randScale) + 1 // prevent divide by 0
				By("Loop " + strconv.Itoa(i))
				// Will scale to a minimum of minPods
				replicas = max(replicas-scaleAmt, minPods)
				fmt.Fprintf(GinkgoWriter, "Scaling cluster down to %v pods by subtracting %v pods\n", replicas,
					scaleAmt)
				quickScale(replicas)
				Expect(replicas).To(Equal(busyboxPodCnt()))
				i++
			}

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
