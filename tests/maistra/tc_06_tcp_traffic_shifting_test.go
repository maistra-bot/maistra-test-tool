// Copyright 2019 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package dashboard provides testing of the grafana dashboards used in Istio
// to provide mesh monitoring capabilities.

package maistra

import (
	"strings"
	"testing"
	"time"

	"istio.io/istio/pkg/log"
	"istio.io/istio/tests/util"
)


func cleanup06(namespace, kubeconfig string) {
	log.Infof("# Cleanup. Following error can be ignored...")
	util.KubeDelete(namespace, echoAllv1Yaml, kubeconfig)
	util.KubeDelete(namespace, echoYaml, kubeconfig)
	log.Info("Waiting for rules to be cleaned up. Sleep 10 seconds...")
	time.Sleep(time.Duration(10) * time.Second)
}


func deployEcho(namespace, kubeconfig string) error {
	log.Infof("# Deploy tcp-echo")
	if err := util.KubeApply(namespace, echoYaml, kubeconfig); err != nil {
		return err
	}
	log.Info("Waiting for rules to propagate. Sleep 10 seconds...")
	time.Sleep(time.Duration(10) * time.Second)
	
	if err := util.CheckPodRunning(testNamespace, "app=tcp-echo", ""); err != nil {
		return err
	}
	log.Info("Waiting for rules to propagate. Sleep 10 seconds...")
	time.Sleep(time.Duration(10) * time.Second)
	return nil
}

func routeTrafficAllv1(namespace, kubeconfig string) error {
	log.Info("Route all TCP traffic to v1 echo")
	if err := util.KubeApply(namespace, echoAllv1Yaml, kubeconfig); err != nil {
		return err
	}
	log.Info("Waiting for rules to propagate. Sleep 10 seconds...")
	time.Sleep(time.Duration(10) * time.Second)
	return nil
}

func routeTraffic20v2(namespace, kubeconfig string) error {
	log.Info("Route 20% of the traffic to v2 echo")
	if err := util.KubeApply(namespace, echo20v2Yaml, kubeconfig); err != nil {
		return err
	}
	log.Info("Waiting for rules to propagate. Sleep 10 seconds...")
	time.Sleep(time.Duration(10) * time.Second)
	return nil
}

func checkEcho(ingressHost, ingressTCPPort string) (string, error) {
	msg, err := util.ShellSilent("docker run -e INGRESS_HOST=%s -e INGRESS_PORT=%s --rm busybox sh -c \"(date; sleep 1) | nc %s %s\"",
				ingressHost, ingressTCPPort, ingressHost, ingressTCPPort)
	if err != nil {
		return "", err
	}
	return msg, nil
}


func Test06(t *testing.T) {
	log.Infof("# TC_06 TCP Traffic Shifting")
	ingressHostIP, err := GetIngressHostIP("")
	Inspect(err, "cannot get ingress host ip", "", t)
	
	ingressTCPPort, err := GetTCPIngressPort("istio-system", "istio-ingressgateway", "")
	Inspect(err, "cannot get ingress TCP port", "", t)

	Inspect(deployEcho(testNamespace, ""), "failed to apply rules", "", t)

	t.Run("100%_v1_shift", func(t *testing.T) {
		log.Info("# Shifting all TCP traffic to v1")
		Inspect(routeTrafficAllv1(testNamespace, ""), "failed to apply rules", "", t)
		tolerance := 0.0
		totalShot := 100
		versionCount := 0
		
		log.Infof("Waiting for checking echo dates. Sleep %d seconds...", totalShot)

		for i := 0; i < totalShot; i++ {
			msg, err := checkEcho(ingressHostIP, ingressTCPPort)
			Inspect(err, "faild to get date", "", t)
			if strings.Contains(msg, "one") {
				versionCount++
			} else {
				log.Errorf("unexpected echo version: %s", msg)
			}
		}

		if isWithinPercentage(versionCount, totalShot, 1, tolerance) {
			log.Info("Success. TCP Traffic shifting acts as expected for 100 percent.")
		} else {
			t.Errorf(
				"Failed traffic shifting test for 100 percent. " +
				"Expected version hit %d", versionCount)
		}
	})

	t.Run("20%_v2_shift", func(t *testing.T) {
		log.Info("# Shifting 20% TCP traffic to v2 tolerance 10% ")
		Inspect(routeTraffic20v2(testNamespace, ""), "failed to apply rules", "", t)
		tolerance := 0.10
		totalShot := 100
		c1, c2 := 0, 0

		log.Infof("Waiting for checking echo dates. Sleep %d seconds...", totalShot)

		for i := 0; i < totalShot; i++ {
			msg, err := checkEcho(ingressHostIP, ingressTCPPort)
			Inspect(err, "failed to get date", "", t)
			if strings.Contains(msg, "one") {
				c1++
			} else if strings.Contains(msg, "two") {
				c2++
			} else {
				log.Errorf("unexpected echo version: %s", msg)
			}
		}

		if isWithinPercentage(c1, totalShot, 0.8, tolerance) && isWithinPercentage(c2, totalShot, 0.2, tolerance) {
			log.Infof("Success. Traffic shifting acts as expected. " +
			"v1 version hit %d, v2 version hit %d", c1, c2)
		} else {
			t.Errorf("Failed traffic shifting test for 20 percent. " +
			"v1 version hit %d, v2 version hit %d", c1, c2)
		}
	})

	defer cleanup06(testNamespace, "")
	defer func() {
		// recover from panic if one occured. This allows cleanup to be executed after panic.
		if err := recover(); err != nil {
			log.Infof("Test failed: %v", err)
		}
	}()
}