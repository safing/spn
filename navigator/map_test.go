package navigator

import (
	"net"
	"os"
	"runtime/pprof"
	"sync"
	"testing"
	"time"

	"github.com/safing/spn/hub"

	"github.com/safing/portmaster/intel/geoip"
)

func TestMap(t *testing.T) {

	finished := make(chan struct{})
	go func() {
		// wait for test to complete, panic after timeout
		time.Sleep(3 * time.Second)
		select {
		case <-finished:
		default:
			t.Log("===== TAKING TOO LONG FOR TEST - PRINTING STACK TRACES =====")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			os.Exit(1)
		}
	}()

	collection := buildTestNet()
	collection["a"] = NewPort(&hub.Hub{
		ID: "a",
		Info: &hub.HubAnnouncement{
			ID:   "a",
			IPv4: net.ParseIP("6.0.0.1"),
		},
		Status: &hub.HubStatus{},
	})

	var lock sync.Mutex

	m := NewMap(collection["1"], collection, &lock)

	testNearestPort(t, m, nil, "104.103.72.43")
	return

	// IPv4
	testNearestPort(t, m, nil, "54.132.253.167")
	testNearestPort(t, m, nil, "208.32.149.85", "120.66.49.150")
	testNearestPort(t, m, nil, "36.50.197.219", "122.214.8.60", "101.180.149.71")
	testNearestPort(t, m, nil, "252.218.156.118")
	testNearestPort(t, m, nil, "19.222.178.108", "240.68.18.10")
	testNearestPort(t, m, nil, "240.123.37.25", "154.174.117.119", "88.198.38.151")
	testNearestPort(t, m, nil, "136.163.102.81")
	testNearestPort(t, m, nil, "115.52.249.186", "235.217.36.133")
	testNearestPort(t, m, nil, "0.220.189.54", "37.25.96.161", "160.8.47.70")
	testNearestPort(t, m, nil, "211.227.246.152")
	testNearestPort(t, m, nil, "155.112.205.226", "65.167.50.34")
	testNearestPort(t, m, nil, "208.112.90.198", "38.62.167.39", "154.46.136.107")
	testNearestPort(t, m, nil, "59.89.247.101")
	testNearestPort(t, m, nil, "172.55.219.14", "13.145.16.109")
	testNearestPort(t, m, nil, "196.1.29.138", "52.224.175.57", "72.49.21.47")

	// IPv6
	testNearestPort(t, m, nil, "2a00:1298:8016::9e10:8c:e26f:e5be")
	testNearestPort(t, m, nil, "2a00:1338:40:c739:5711:9554:b193:155", "2a02:124:1001:8:e7ab:a466:52e7:f6da")
	testNearestPort(t, m, nil, "2a02:188:1000:e50e:6f15:5b55:60e:c3ab", "2605:6280:1c2b:7b80:f492:a34e:c2cf:65b7", "2001:4871:f93c:7d2f:4564:eda:1388:cb3b")

	testNearestPort(t, m, nil, "2a01:490:de62:b9f0:5a8d:25f8:bff5:b3bc")
	testNearestPort(t, m, nil, "2402:ef02:457a:1287:dace:8cfc:36da:9455", "2001:16b0:4365:145d:eae7:d5b0:60b1:5a00")
	testNearestPort(t, m, nil, "2a03:1ac0:b0d4:1000:94d9:9b47:4302:bd78", "2620:111:800a:24fa:fa01:2777:a78f:e13b", "2403:e800:e800:40:2f3d:c69f:80d7:f5fd")
	testNearestPort(t, m, nil, "2406:3100:69c6:13fe:6236:ec5a:a4f3:b809")
	testNearestPort(t, m, nil, "2406:4300:366:d0e3:de5f:8f27:1cdb:4425", "2406:5a00:a524:e75c:4028:10c6:fbb1:2f81")
	testNearestPort(t, m, nil, "2406:5e00:5:4a02:4e4b:61e:54d6:d5c5", "2406:6600:8ea5:7796:c8a0:cb15:7398:4fe2", "2a03:8160:1:27ca:508:afa6:ee68:eff9")
	testNearestPort(t, m, nil, "2a03:8180:1800:cb1f:9d30:ea39:9b8b:f04e")
	testNearestPort(t, m, nil, "2a03:8620:f34b:31cf:6282:e46d:2211:4199", "2800:1f0:8000:e573:daf6:ca37:63d3:7b8")
	testNearestPort(t, m, nil, "2800:26e:4da1:e9e1:c68f:d0d4:453d:888a", "2800:300:9af:fff7:39d5:b46:6c4c:d38b", "2800:5e0:bee1:8455:ee12:38ea:d22:3d57")
	testNearestPort(t, m, nil, "2405:8400:7c16:6438:a589:1c7b:6566:5e67")
	testNearestPort(t, m, nil, "2405:8900:fc9c:5dd3:f6f1:b86b:2ee8:2fbf", "2405:9000:1000:6b50:d07e:3354:4995:7ab7")
	testNearestPort(t, m, nil, "2405:a000:b455:23ba:feea:13cc:a69e:92c5", "2405:a700:b0c6:b87d:be6b:e662:ee36:c0b4", "2405:ba00:8000:1726:589b:b10f:70b2:2111")

	testNearestPort(t, m, nil, "2406:f00:1:4968:1900:6483:226a:bd7e", "88.169.166.181", "236.12.137.238")
	testNearestPort(t, m, nil, "2406:1d00:1849:af0c:b35:595a:6b45:ad3f", "2406:2e00:1379:7028:fd19:7858:b2d2:b733", "99.234.160.85")
	testNearestPort(t, m, nil, "207.234.132.111", "2406:3b00:b9fe:1ed2:cf1:c948:5119:8819", "47.194.191.175")
	testNearestPort(t, m, nil, "62.218.230.50", "29.217.212.59", "2a0a:54c0:7281:363:ca5:af28:5547:c10d")
	testNearestPort(t, m, nil, "2a0a:5d40:633:95c4:e6a4:d17e:367d:b7be", "161.6.154.198", "2a0a:6700:50be:b3e7:23e4:91e6:e06:c2ce")
	testNearestPort(t, m, nil, "37.75.9.180", "2afb:dac6:d4ff:ad0b:d45a:a45c:4514:7583", "126.127.20.158")
	testNearestPort(t, m, nil, "2a0a:6fc0:1253:8ad7:31f9:471f:980c:5c88", "225.74.37.147", "94.218.163.132")
	testNearestPort(t, m, nil, "131.1.119.22", "2003:3c0:1603:10:343f:548e:a41f:585", "2003:1c00:9011:b285:21f7:3cd6:2635:1db9")
	testNearestPort(t, m, nil, "2400:1700:8000:7931:6620:c3c5:4f85:43ae", "38.126.217.120", "2400:2000:0:200:3025:831a:efe0:5e65")
	testNearestPort(t, m, nil, "2607:7c80:55:bd84:d217:1356:46c2:9444", "28.128.140.103", "2607:8280:5:5945:a35e:38b5:7527:c5ad")

	close(finished)

	// let all the logs get out before we might fail
	// time.Sleep(100 * time.Millisecond)

}

func testNearestPort(t *testing.T, m *Map, expectedPorts []uint8, dests ...string) {
	var ips []net.IP
	for _, dest := range dests {
		ip := net.ParseIP(dest)
		if ip == nil {
			t.Errorf("could not parse IP %s", dest)
		} else {
			ips = append(ips, ip)
			loc, err := geoip.GetLocation(ip)
			if err != nil {
				t.Logf("could not get geoip for %s", ip)
			} else {
				t.Logf("location of %s: %v", ip, loc)
			}
		}
	}

	col, err := m.FindNearestPorts(ips)
	if err != nil {
		t.Errorf("error finding nearest port: %s", err)
	}
	if col.Len() == 0 {
		t.Errorf("no ports found near %s", ips)
	}

	t.Logf("===== results for %v", ips)
	for _, port := range col.All {
		t.Logf("%v", port)
	}
}
