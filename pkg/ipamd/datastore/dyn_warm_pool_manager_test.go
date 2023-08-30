package datastore

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var (
	now           = time.Now()
	before30min   = now.Truncate(time.Hour).Add(-time.Minute * 30)
	before35min   = now.Truncate(time.Hour).Add(-time.Minute * 35)
	before36min   = now.Truncate(time.Hour).Add(-time.Minute * 36)
	before1hour   = now.Truncate(time.Hour).Add(-time.Hour * 1)
	before2hour   = now.Truncate(time.Hour).Add(-time.Hour * 2)
	before3hour   = now.Truncate(time.Hour).Add(-time.Hour * 3)
	before4hour   = now.Truncate(time.Hour).Add(-time.Hour * 4)
	before6hour   = now.Truncate(time.Hour).Add(-time.Hour * 6)
	before6hour2  = now.Truncate(time.Hour).Add(-time.Hour*6 + -time.Minute*30)
	before8hour   = now.Truncate(time.Hour).Add(-time.Hour * 8)
	before10hour  = now.Truncate(time.Hour).Add(-time.Hour * 10)
	before18hour  = now.Truncate(time.Hour).Add(-time.Hour * 18)
	before22hour  = now.Truncate(time.Hour).Add(-time.Hour * 22)
	before23hour  = now.Truncate(time.Hour).Add(-time.Hour * 23)
	before23hour2 = now.Truncate(time.Hour).Add(-time.Hour*22 + -time.Minute*30)
	before23hour3 = now.Truncate(time.Hour).Add(-time.Hour*22 + -time.Minute*55)
	before23hour4 = now.Truncate(time.Hour).Add(-time.Hour*22 + -time.Minute*59)
	after24       = now.Truncate(time.Hour).Add(-time.Hour * 24)
	after242      = now.Truncate(time.Hour).Add(-time.Hour*24 + -time.Minute*1)
	after243      = now.Truncate(time.Hour).Add(-time.Hour*24 + -time.Second*30)
	initinUseIPs1 = 0
	initinUseIPs2 = 20
)

func TestNewDynamicWarmPoolManager(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	assert.Equal(t, 0, len(m.inUseIPHistory))
	assert.Equal(t, 0, m.inUseIPs)
}

func TestGarbageCollectHistory(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	lastGarbageCollect := m.lastGarbageCollect
	assert.Equal(t, 0, len(m.inUseIPHistory))
	m.RecordIPAllocation(after242)
	m.garbageCollectHist()
	assert.Equal(t, lastGarbageCollect, m.lastGarbageCollect)
	assert.Equal(t, 0, len(m.inUseIPHistory))
	m.RecordIPAllocation()
	assert.Equal(t, lastGarbageCollect, m.lastGarbageCollect)
	assert.Equal(t, 1, len(m.inUseIPHistory))
}

func TestRecordIPAllocation(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.RecordIPAllocation()
	assert.Equal(t, 1, len(m.inUseIPHistory))
	assert.Equal(t, 1, m.inUseIPHistory[0].inUseIPs)
	assert.Equal(t, 1, m.inUseIPHistory[0].op)
}

func TestRecordIPDeallocation(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.RecordIPDeallocation()
	assert.Equal(t, 1, len(m.inUseIPHistory))
	assert.Equal(t, 0, m.inUseIPHistory[0].inUseIPs)
	assert.Equal(t, -1, m.inUseIPHistory[0].op)
}

func TestInsertInUseIPHistory(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)

	m.RecordIPAllocation(before23hour4)
	m.RecordIPAllocation(before23hour3)
	m.RecordIPAllocation(before23hour2)
	m.RecordIPAllocation(before23hour)

	assert.Equal(t, 4, len(m.inUseIPHistory))
	assert.Equal(t, 4, m.inUseIPs)

	m.RecordIPAllocation(before22hour)
	m.RecordIPDeallocation()

	assert.Equal(t, 6, len(m.inUseIPHistory))
	assert.Equal(t, 4, m.inUseIPs)

	m.RecordIPAllocation(before18hour)
	m.RecordIPAllocation(before10hour)
	m.RecordIPAllocation(before8hour)
	m.RecordIPDeallocation(before6hour2)
	m.RecordIPAllocation(before6hour)

	assert.Equal(t, 11, len(m.inUseIPHistory))
	assert.Equal(t, 7, m.inUseIPs)

	m.RecordIPAllocation(before2hour)
	m.RecordIPAllocation(before1hour)
	m.RecordIPAllocation(before35min)
	m.RecordIPAllocation(before36min)
	m.RecordIPAllocation(before30min)

	assert.Equal(t, 18, len(m.inUseIPHistory))
	assert.Equal(t, 14, m.inUseIPs)
}

func TestNetChangeOver(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.RecordIPAllocation(before35min)
	m.RecordIPAllocation(before36min)
	m.RecordIPAllocation(before30min)

	netChange := m.netChangeOver(before1hour, time.Now())
	assert.Equal(t, 3, netChange)
}

func TestNetChangeOver2(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.RecordIPAllocation(before4hour)
	m.RecordIPAllocation(before3hour)
	m.RecordIPAllocation(before35min)
	m.RecordIPAllocation(before36min)
	m.RecordIPAllocation(before30min)

	netChange := m.netChangeOver(before4hour, before1hour)
	assert.Equal(t, 2, netChange)

	netChange = m.netChangeOver(before1hour, now)
	assert.Equal(t, 3, netChange)

	netChange = m.netChangeOver(before3hour, now)
	assert.Equal(t, 4, netChange)
}

func TestNetChangeOverNoHistory(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)

	m.RecordIPAllocation(after242)
	m.RecordIPAllocation(after24)
	m.RecordIPAllocation(after243)
	m.garbageCollectHist()

	// garbage collected entire history
	netChange := m.netChangeOver(before1hour, time.Now())
	assert.Equal(t, 0, netChange)
	assert.Equal(t, 3, m.inUseIPs)
}

func TestNetChangeOverNewCluster(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)

	netChange := m.netChangeOver(time.Now(), time.Now())
	assert.Equal(t, 0, netChange)
}

func TestNetChangeOverSingleEntry(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.RecordIPAllocation()
	netChange := m.netChangeOver(now, now.Add(time.Hour))
	assert.Equal(t, 1, netChange)
}

func TestNetChangeOverHist(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	netMap := m.netChangeOverHist()
	m.RecordIPAllocation()
	assert.Equal(t, 1, m.inUseIPs)
	netMap = m.netChangeOverHist()
	assert.Equal(t, 1, netMap[0])
	assert.Equal(t, 0, netMap[before1hour.Hour()])
	assert.Equal(t, 0, netMap[before2hour.Hour()])
}

func TestNetChangeOverHist1(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{after24, 1, 1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{after24, 2, 1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{after242, 3, 1})
	m.RecordIPAllocation()
	netArr := m.netChangeOverHist()
	assert.Equal(t, 4, m.inUseIPs)
	assert.Equal(t, 1, netArr[0])

	fmt.Println(netArr)
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before23hour4, 5, 1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before23hour3, 6, 1})
	m.inUseIPs--
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before23hour2, 5, -1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before23hour, 6, 1})
	m.inUseIPs--
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before22hour, 5, -1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before18hour, 6, 1})
	m.inUseIPs--
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before10hour, 5, -1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before8hour, 6, 1})
	m.inUseIPs--
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before6hour, 5, -1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before4hour, 6, 1})
	m.inUseIPs--
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before3hour, 5, -1})
	m.inUseIPs++
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before2hour, 6, 1})
	m.inUseIPs--
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{before1hour, 5, -1})

	netArr = m.netChangeOverHist()
	fmt.Println(netArr, m.inUseIPs, before18hour)
	assert.Equal(t, 5, m.inUseIPs)
	assert.Equal(t, 1, netArr[now.Hour()])
	assert.Equal(t, -1, netArr[1])
	assert.Equal(t, 1, netArr[2])
	assert.Equal(t, 2, netArr[23])
}

func TestGetWarmIPTarget(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)

	fmt.Println("STATS       AVG |        STDDEV       | P75               | WARM POOL TARGET")

	netArr := []int{0, 0, 0, 0, 0, 0, 1, -1, 1, -1, 0, -1, 1, 0, -1, 1, 0, -1, 1, -1, 1, -1, 0, 1}
	m.inUseIPs = 1
	warmIPTarget := m.GetWarmIPTarget(netArr)
	avg, stdDev := m.netStdDev(netArr)
	p75 := m.netP75(netArr)
	fmt.Println("         | ", avg, " | ", stdDev, "| ", p75, " | ", warmIPTarget)

	// net sum of
	netArr = []int{3, 6, 7, -4, 1, -1, 3, 2, 1, 0, -3, -4, 5, -2, -5, 7, -1, 1, 2, 4, 1, -1, 1, 1}
	m.inUseIPs = 5
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of
	netArr = []int{5, -3, 7, -4, 1, -1, 3, 2, 1, 0, -3, 0, 5, -2, -5, 7, 0, 0, 2, 4, 1, -1, 0, 1}
	m.inUseIPs = 5
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 20
	netArr = []int{0, 0, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 0
	netArr = []int{0, 0, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -20, 0, 0, 0}
	m.inUseIPs = 1
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 0
	netArr = []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	m.inUseIPs = 1
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 0
	netArr = []int{0, 0, 20, 15, 10, -45, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -20, 0, 0, 0}
	m.inUseIPs = 5
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 0
	netArr = []int{0, 0, 20, -15, 10, -5, 0, 0, 0, 30, -30, 0, 0, 0, 5, -5, 0, 0, 0, 20, -20, 0, 10, 0}
	m.inUseIPs = 2
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 0
	netArr = []int{1, 2, 3, 4}
	m.inUseIPs = 1
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))

	// net sum of 0
	netArr = []int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, -2, 0, 0, 0, 2, 0, 0, 0}
	m.inUseIPs = 0
	m.GetWarmIPTarget(netArr)
	fmt.Println(m.GetWarmIPTarget(netArr))
}

func TestCheckForBursts(t *testing.T) {
	m := NewDynamicWarmPoolManager(initinUseIPs1)
	m.inUseIPs = 15
	m.inUseIPHistory = append(m.inUseIPHistory, &ipEntry{now.Add(-time.Second * 1), 15, 15})
	net := m.CheckForBursts()
	assert.Equal(t, 15, net)
}

func TestNetStdDev(t *testing.T) {

}

func TestNetP75y(t *testing.T) {

}

func TestNetAvg(t *testing.T) {

}
