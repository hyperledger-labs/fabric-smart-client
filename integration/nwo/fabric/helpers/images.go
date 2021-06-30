/*
Copyright IBM Corp All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package helpers

import (
	"fmt"

	docker "github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func AssertImagesExist(imageNames ...string) {
	dockerClient, err := docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	for _, imageName := range imageNames {
		images, err := dockerClient.ListImages(docker.ListImagesOptions{
			Filters: map[string][]string{"reference": {imageName}},
		})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())

		if len(images) != 1 {
			Fail(fmt.Sprintf("missing required image: %s", imageName), 1)
		}
	}
}
