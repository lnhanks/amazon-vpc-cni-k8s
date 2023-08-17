// VPC Warm Pool Test Suite
// This test suite is a foundation for evaluating a dynamic warm pool, or ip consumption in general. Pair with grafana
//metrics dashboard to look at ip allocation and consumption. Each test displays the warm pool environment variables
//before and after to evaluate the changes made to the warm pool. Environment variables are not reset before and after
//each test so that way multiple tests can be run to evaluate behavior. You can run the test "clear env" which will
//unset all warm pool environment variables. Or, if you  want to test the behavior with some of those environment
//variables set, alter them in that test and run it once before you run the desired tests.
// Use Case Test 1: Quick Scale Up and Down
// Use Case Test 2: Sawtooth Fixed Add and Subtract
// Use Case Test 3: Random Scale Fixed Add and Subtract
// Use Case Test 4: Random Scale Random Add and Subtract Operations
// Use Case Test 5: Proportionate Scaling
// Use Case Test 6: Random Scaling
// Use Case Test 7: Single Burst Behavior
// Use Case Test 8: Multiple Burst Behavior
// Use Case Test 9: Random Add to Max, Random Sub to Min

package warm_pool

import (
	"fmt"
	"github.com/aws/amazon-vpc-cni-k8s/test/framework/resources/k8s/manifest"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"math/rand"
	"testing"
	"time"

	"github.com/aws/amazon-vpc-cni-k8s/test/framework"
	k8sUtils "github.com/aws/amazon-vpc-cni-k8s/test/framework/resources/k8s/utils"
	"github.com/aws/amazon-vpc-cni-k8s/test/framework/utils"
)

// Warm Pool Test Suite Constants
// Run all tests with these constants or change individual tests to get desired outcome
const (
	randDigits      = 10   // exclusive, used in rand.Intn to change scale amount, <= maxPods, (3,6,9)
	scale           = 0.25 // used in set proportional scaling, iterate with a fixed percentage (5)
	iterations      = 10   // run test over a set number of iterations (2,3,4,7,8)
	iterPods        = 1    // iterate with a fixed number of pods (2,7,8)
	numBursts       = 2    // Use Case Test 8, set number of bursts (8)
	preventNoChange = 0    // retries x amount of times if randInt/randOp is out of range, if out of range no cluster
	// scaling occurs, if set above 0 will increment some areas of no cluster scaling (3, 4, 6, 8, 9)
	maxPods = 100             // max pods you want to work with for your cluster (all)
	minPods = 0               // tests can be run with a base amount of pods at start (all)
	sleep   = 1 * time.Minute // sleep interval, make sure it is long enough to allow scaling (all)
)

var primaryInstance *ec2.Instance
var f *framework.Framework
var err error
var coreDNSDeploymentCopy *v1.Deployment

const CoreDNSDeploymentName = "coredns"
const KubeSystemNamespace = "kube-system"

func TestWarmPool(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "VPC Warm Pool Test Suite")
}

var _ = BeforeSuite(func() {
	f = framework.New(framework.GlobalOptions)

	By("creating test namespace")
	f.K8sResourceManagers.NamespaceManager().
		CreateNamespace(utils.DefaultTestNamespace)

	nodeList, err := f.K8sResourceManagers.NodeManager().GetNodes(f.Options.NgNameLabelKey,
		f.Options.NgNameLabelVal)
	Expect(err).ToNot(HaveOccurred())

	numOfNodes := len(nodeList.Items)
	Expect(numOfNodes).Should(BeNumerically(">", 1))

	// Nominate the first untainted node as the one to run coredns deployment against
	By("adding nodeSelector in coredns deployment to be scheduled on single node")
	var primaryNode *corev1.Node
	for _, n := range nodeList.Items {
		if len(n.Spec.Taints) == 0 {
			primaryNode = &n
			break
		}
	}
	Expect(primaryNode).To(Not(BeNil()), "expected to find a non-tainted node")
	instanceID := k8sUtils.GetInstanceIDFromNode(*primaryNode)
	primaryInstance, err = f.CloudServices.EC2().DescribeInstance(instanceID)
	Expect(err).ToNot(HaveOccurred())

	By("getting node with no pods scheduled to run tests")
	coreDNSDeployment, err := f.K8sResourceManagers.DeploymentManager().GetDeployment(CoreDNSDeploymentName,
		KubeSystemNamespace)
	Expect(err).ToNot(HaveOccurred())

	// Copy the deployment to restore later
	coreDNSDeploymentCopy = coreDNSDeployment.DeepCopy()

	// Add nodeSelector label to coredns deployment so coredns pods are scheduled on 'primary' node
	coreDNSDeployment.Spec.Template.Spec.NodeSelector = map[string]string{
		"kubernetes.io/hostname": primaryNode.Labels["kubernetes.io/hostname"],
	}
	err = f.K8sResourceManagers.DeploymentManager().UpdateAndWaitTillDeploymentIsReady(coreDNSDeployment,
		utils.DefaultDeploymentReadyTimeout)
	Expect(err).ToNot(HaveOccurred())

	// Redefine primary node as node without coredns pods. Note that this node may have previously had coredns pods.
	for _, n := range nodeList.Items {
		if len(n.Spec.Taints) == 0 && n.Name != primaryNode.Name {
			primaryNode = &n
			break
		}
	}
	fmt.Fprintf(GinkgoWriter, "primary node is %s\n", primaryNode.Name)
	instanceID = k8sUtils.GetInstanceIDFromNode(*primaryNode)
	primaryInstance, err = f.CloudServices.EC2().DescribeInstance(instanceID)
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	// Restore coredns deployment
	By("restoring coredns deployment")
	err = f.K8sResourceManagers.DeploymentManager().UpdateAndWaitTillDeploymentIsReady(coreDNSDeploymentCopy,
		utils.DefaultDeploymentReadyTimeout)

	By("deleting test namespace")
	f.K8sResourceManagers.NamespaceManager().
		DeleteAndWaitTillNamespaceDeleted(utils.DefaultTestNamespace)
})

// Helper Functions //
func getWarmPoolEnvVars() {
	daemonset, _ := f.K8sResourceManagers.DaemonSetManager().GetDaemonSet("kube-system", "aws-node")
	warmPoolKeys := [4]string{"WARM_ENI_TARGET", "MINIMUM_IP_TARGET", "WARM_IP_TARGET", "WARM_PREFIX_TARGET"}
	print("----\n")
	for _, key := range warmPoolKeys {
		val := utils.GetEnvValueForKeyFromDaemonSet(key, daemonset)
		if val != "" {
			print("  -", key, " : ", val, "\n")
		} else {
			print("  -", key, " : not set", "\n")
		}
	}
	print("----\n")
}

// Random operation, if preventNoChange is 0 this includes no change being a result, otherwise it will add or subtract
func randOp(replicas int, pods int) (int, string) {
	if preventNoChange == 0 {
		op := rand.Intn(3)
		if op == 0 {
			return replicas + pods, "adding"
		}
		if op == 1 {
			return replicas - pods, "subtracting"
		} else {
			return replicas, "no change with"
		}
	} else {
		randPods := rand.Intn(randDigits)
		op := rand.Intn(2)
		if op == 0 {
			return replicas + randPods, "adding"
		} else {
			return replicas - randPods, "subtracting"
		}
	}
}

// Tries to get a random op/number combo that actually changes the cluster. If preventNoChange is above 0, will
// attempt to get another random integer to add/subtract that is within range. This is not always possible depending on
// what iterations and randDigits is set to, so it is best to set preventNoChange to a low number if it is set at all.
// If you want to see periods of no change, set this to 0.
func randOpLoop(replicas int) (int, string, int) {
	result := 0
	op := ""
	randPods := 0
	for i := 0; i < preventNoChange+1; i++ {
		randPods := rand.Intn(randDigits)
		result, op := randOp(replicas, randPods)
		if result > minPods && result < maxPods && randPods != 0 {
			return result, op, randPods
		}
	}
	return result, op, randPods
}

func quickScale(pods int) {
	deploymentSpec := manifest.NewBusyBoxDeploymentBuilder(f.Options.TestImageRegistry).
		Namespace("default").
		Name("busybox").
		NodeName(primaryNode.Name).
		Namespace(utils.DefaultTestNamespace).
		Replicas(pods).
		Build()

	err := f.K8sResourceManagers.
		DeploymentManager().
		UpdateAndWaitTillDeploymentIsReady(deploymentSpec, utils.DefaultDeploymentReadyTimeout*5)
	Expect(err).ToNot(HaveOccurred())

	time.Sleep(sleep)
}

// Check on pod count outside deployment
func busyboxPodCnt() int {
	podCount := 0
	podList, _ := f.K8sResourceManagers.PodManager().GetPodsWithLabelSelector("role", "test")
	for _, _ = range podList.Items {
		podCount += 1
	}
	return podCount
}

func checkInRange(result int) int {
	replicas := result
	replicas = max(replicas, minPods)
	replicas = min(replicas, maxPods)
	return replicas
}

// Tries to prevent no scaling in the cluster as rand.Intn is inclusive with 0, so just scale 1 instead.
func incIf(pods int) int {
	if pods == 0 && preventNoChange > 0 {
		return 1
	} else {
		return pods
	}
}

func max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func min(x, y int) int {
	if y < x {
		return y
	}
	return x
}
