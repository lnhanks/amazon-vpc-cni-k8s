package datastore

import (
	"github.com/aws/amazon-vpc-cni-k8s/pkg/utils/logger"
	"math"
	"sort"
	"time"
)

// README:
// This manager controls the warm pool, when enabled, by providing a warm pool value that adapts based on ip allocation
// histories. This can be enabled for clusters running iPv4 without prefix mode as a way to optimize the number of ENIs
// per node to prevent over provisioning and lessen cost to the consumer while still providing a warm pool amount that
// can respond to cluster scaling. This file contains a structure and helper functions that can easily be adapted into a
// better, more robust algorithm as necessary. The associated unit tests, Ginkgo test suite warm_pool, and Grafana
// dashboard provide further tooling to do so.
//
// The current algorithm:
//  1. Gets the standard deviation and average of the net ip request history over the past 24 hours
//  2. If the standard deviation is greater than 5, use the max value of:
//     the p75 of the net ip request over the past 24 hours
//     the average + standard deviation
//  3. If the standard deviation is less than 5, use the average + standard deviation
//  4. Check for bursty behavior by looking at recent activity, use the max value of:
//     the max ip requests over the past 30 minutes
//     calculated value
//  5. Check for minimal activity ie. 0 net requests over the past 24 hours, no bursts, and use the max value of:
//     2
//     calculated value

type ipEntry struct {
	timestamp time.Time
	inUseIPs  int
	op        int
}

type inUseIPHistory []*ipEntry

// Keeps track of the inUseIPs, which parellels assigned from datastore, to keep an accurate ip allocation history.
type DynamicWarmPoolManager struct {
	inUseIPs           int
	inUseIPHistory     inUseIPHistory
	lastGarbageCollect time.Time
	log                logger.Logger
}

func NewDynamicWarmPoolManager(log logger.Logger, inUseIPs int) *DynamicWarmPoolManager {
	return &DynamicWarmPoolManager{
		inUseIPs:           inUseIPs,
		inUseIPHistory:     inUseIPHistory{},
		lastGarbageCollect: time.Now(),
		log:                log,
	}
}

// RecordIPAllocation is called in datastore when an IP is requested successfully. It increments the inUseIPs and adds
// an ipEntry to the inUseIPHistory
func (m *DynamicWarmPoolManager) RecordIPAllocation(inputTimestamp ...time.Time) {
	var timestamp time.Time
	// variadic argument
	if len(inputTimestamp) < 1 {
		timestamp = time.Now()
	} else {
		timestamp = inputTimestamp[0]
	}

	m.inUseIPs = m.inUseIPs + 1
	ipEntry := ipEntry{timestamp, m.inUseIPs, 1}
	m.garbageCollectHist()
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry)
}

// RecordIPDeallocation is called in datastore when an IP is requested successfully to be deallocated. It decrements
// the inUseIPs and adds an ipEntry to the inUseIPHistory
func (m *DynamicWarmPoolManager) RecordIPDeallocation(inputTimestamp ...time.Time) {
	var timestamp time.Time
	// variadic argument
	if len(inputTimestamp) < 1 {
		timestamp = time.Now()
	} else {
		timestamp = inputTimestamp[0]
	}

	m.inUseIPs = m.max(m.inUseIPs-1, 0)
	ipEntry := ipEntry{timestamp, m.inUseIPs, -1}
	m.garbageCollectHist()
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry)
}

// GetWarmIPTarget is called in ipamd if the dynamic warm pool is enabled. Operates with no arguments and has a
// variadic argument for testing purposes only. Calculates the dynamic warm pool target by using the algorithm described
// in the README above.
func (m *DynamicWarmPoolManager) GetWarmIPTarget(inputNetArray ...[]int) int {
	var netArr []int
	// variadic argument
	if len(inputNetArray) < 1 {
		netArr = m.netChangeOverHist()
	} else {
		netArr = inputNetArray[0]
	}

	stdDev, avg := m.netStdDev(netArr)
	netP75 := m.netP75(netArr)
	burst := m.maxOver(time.Now(), time.Now().Add(-time.Minute*30))

	warmTarget := stdDev + avg

	if stdDev > 5 {
		warmTarget = m.max(netP75, warmTarget)
	}
	warmTarget = m.max(warmTarget, burst)
	warmTarget = m.max(warmTarget, 2)
	log.Debugf("Dynamic Warm Pool-Setting warm IP target to : target: %d", warmTarget)
	log.Debugf("Net ip request history per hour %v", netArr)
	return warmTarget
}

// CheckForBursts evaluates for bursty behavior by looking at the net ip request behavior over the past hour
func (m *DynamicWarmPoolManager) CheckForBursts() int {
	now := time.Now()
	hourAgo := now.Add(-time.Hour * 1)
	net := m.netChangeOver(hourAgo, now)
	return net
}

// netChangeOver gets the net ip requests over arguments start and end
func (m *DynamicWarmPoolManager) netChangeOver(start time.Time, end time.Time) int {
	net := 0
	for _, ipEntry := range m.inUseIPHistory {
		if ipEntry.timestamp.Unix() >= start.Unix() && ipEntry.timestamp.Unix() < end.Unix() {
			net += ipEntry.op
		}
	}
	return net
}

// maxOver gets the max ip requests over arguments start and end
func (m *DynamicWarmPoolManager) maxOver(start time.Time, end time.Time) int {
	copyInUseIPHist := make(inUseIPHistory, len(m.inUseIPHistory))
	copy(copyInUseIPHist, m.inUseIPHistory)

	// sort by timestamp to get ops in order
	sort.Slice(copyInUseIPHist, func(i, j int) bool {
		return m.inUseIPHistory[i].timestamp.Unix() < m.inUseIPHistory[j].timestamp.Unix()
	})

	curMax := 0
	tempMax := 0
	for _, ipEntry := range copyInUseIPHist {
		if ipEntry.timestamp.Unix() <= start.Unix() && ipEntry.timestamp.Unix() > end.Unix() {
			tempMax += ipEntry.op
			curMax = m.max(tempMax, curMax)
		}
	}
	return curMax
}

// netChangeOverHist gets the net ip requests per hour over the past 24 hours and returns an array with these values.
// The most recent activity will be stored at the beginning on the array regardless of the current time i.e. activity
// in the past hour will be saved at index 0.
func (m *DynamicWarmPoolManager) netChangeOverHist() []int {
	var netArr []int
	// if last garbage collection was 24+ hours ago, clean up the inUseIPHist before preparing the data
	if time.Now().Sub(m.lastGarbageCollect) > time.Hour*24 {
		m.garbageCollectHist()
		m.lastGarbageCollect = time.Now()
	}
	for i := 0; i < 24; i++ {
		start := time.Now().Add(-time.Hour * time.Duration(i))
		end := time.Now().Add(-time.Hour * time.Duration(i-1))
		netHour := m.netChangeOver(start.Truncate(time.Hour), end.Truncate(time.Hour))
		netArr = append(netArr, netHour)
	}
	return netArr
}

// netAvg gets the average value of the argument array, returns a float to be used to calculate stdDev
func (m *DynamicWarmPoolManager) netAvg(netArr []int) float64 {
	var sum int
	for _, val := range netArr {
		sum += val
	}
	avg := float64(sum) / float64(len(netArr))
	return avg
}

// netP75 gets the p75 value of the argument array
func (m *DynamicWarmPoolManager) netP75(netArr []int) int {
	copyNetArr := make([]int, len(netArr))
	copy(copyNetArr, netArr)
	sort.Ints(copyNetArr)
	p75Idx := math.Round(float64(len(copyNetArr)-1) * 0.75)
	return copyNetArr[int(p75Idx)]
}

// netStdDev gets the standard deviation of the argument array, returns the rounded stddev and average of the array
func (m *DynamicWarmPoolManager) netStdDev(netArr []int) (int, int) {
	var stdDev float64
	avg := m.netAvg(netArr)

	for _, val := range netArr {
		stdDev += math.Pow(float64(float64(val)-avg), 2)
	}
	stdDev = math.Round(math.Sqrt(stdDev / float64(len(netArr)-1)))
	return int(stdDev), int(math.Round(avg))
}

// max gets the max value between the two arguments
func (m *DynamicWarmPoolManager) max(val1 int, val2 int) int {
	if val1 > val2 {
		return val1
	}
	return val2
}

// garbageCollectHist will check all values in the inUseIPHistory to make sure they are not older than 24 hours and
// garbage collect the ones that are.
func (m *DynamicWarmPoolManager) garbageCollectHist() {
	// garbage collect inUseIPHistory for entries that are older than 24 hours
	i := 0
	for _, ePtr := range m.inUseIPHistory {
		ipEntry := *ePtr
		if time.Now().Sub(ipEntry.timestamp).Hours() < (time.Hour * 24).Hours() {
			m.inUseIPHistory[i] = ePtr
			i++
		}
	}
	// Set truncated pointers to nil to prevent memory leaks and mark for garbage collection
	for k := i; k < len(m.inUseIPHistory); k++ {
		m.inUseIPHistory[k] = nil
	}
	m.inUseIPHistory = m.inUseIPHistory[:i]
}
