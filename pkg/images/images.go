/*
 Copyright 2021 The KubeSphere Authors.

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

package images

import (
	"fmt"
	"os"
	"strings"
	"time"

	kubekeyapiv1alpha2 "github.com/kubesphere/kubekey/apis/kubekey/v1alpha2"
	"github.com/kubesphere/kubekey/pkg/common"
	"github.com/kubesphere/kubekey/pkg/core/connector"
	"github.com/kubesphere/kubekey/pkg/core/logger"
	"github.com/pkg/errors"
)

const (
	cnRegistry          = "registry.cn-beijing.aliyuncs.com"
	cnNamespaceOverride = "kubesphereio"
)

// Image defines image's info.
type Image struct {
	RepoAddr          string
	Namespace         string
	NamespaceOverride string
	Repo              string
	Tag               string
	Group             string
	Enable            bool
}

// Images contains a list of Image
type Images struct {
	Images []Image
}

// ImageName is used to generate image's full name.
func (image Image) ImageName() string {
	return fmt.Sprintf("%s:%s", image.ImageRepo(), image.Tag)
}

// ImageRepo is used to generate image's repo address.
func (image Image) ImageRepo() string {
	var prefix string

	if os.Getenv("KKZONE") == "cn" {
		if image.RepoAddr == "" || image.RepoAddr == cnRegistry {
			image.RepoAddr = cnRegistry
			image.NamespaceOverride = cnNamespaceOverride
		}
	}

	if image.RepoAddr == "" {
		if image.Namespace == "" {
			prefix = ""
		} else {
			prefix = fmt.Sprintf("%s/", image.Namespace)
		}
	} else {
		if image.NamespaceOverride == "" {
			if image.Namespace == "" {
				prefix = fmt.Sprintf("%s/library/", image.RepoAddr)
			} else {
				prefix = fmt.Sprintf("%s/%s/", image.RepoAddr, image.Namespace)
			}
		} else {
			prefix = fmt.Sprintf("%s/%s/", image.RepoAddr, image.NamespaceOverride)
		}
	}

	return fmt.Sprintf("%s%s", prefix, image.Repo)
}

// PullImages is used to pull images in the list of Image.
func (images *Images) PullImages(runtime connector.Runtime, kubeConf *common.KubeConf) error {
	pullCmd := "docker"
	switch kubeConf.Cluster.Kubernetes.ContainerManager {
	case "crio":
		pullCmd = "crictl"
	case "containerd":
		pullCmd = "crictl"
	case "isula":
		pullCmd = "isula"
	default:
		pullCmd = "docker"
	}

	host := runtime.RemoteHost()

	for _, image := range images.Images {
		switch {
		case host.IsRole(common.Master) && image.Group == kubekeyapiv1alpha2.Master && image.Enable,
			host.IsRole(common.Worker) && image.Group == kubekeyapiv1alpha2.Worker && image.Enable,
			(host.IsRole(common.Master) || host.IsRole(common.Worker)) && image.Group == kubekeyapiv1alpha2.K8s && image.Enable,
			host.IsRole(common.ETCD) && image.Group == kubekeyapiv1alpha2.Etcd && image.Enable:

			logger.Log.Messagef(host.GetName(), "downloading image: %s", image.ImageName())
			if _, err := runtime.GetRunner().SudoCmd(fmt.Sprintf("env PATH=$PATH %s inspecti -q %s", pullCmd, image.ImageName()), false); err == nil {
				logger.Log.Infof("%s pull image %s exists", pullCmd, image.ImageName())
				continue
			}

			if _, err := runtime.GetRunner().SudoCmd(fmt.Sprintf("env PATH=$PATH %s pull %s", pullCmd, image.ImageName()), false); err != nil {
				return errors.Wrap(err, "pull image failed")
			}
		default:
			continue
		}

	}
	return nil
}

type LocalImage struct {
	Filename string
}

type LocalImages []LocalImage

func (i LocalImages) LoadImages(runtime connector.Runtime, kubeConf *common.KubeConf) error {
	loadCmd := "docker"

	host := runtime.RemoteHost()

	for _, image := range i {
		switch {
		case host.IsRole(common.Master):

			logger.Log.Messagef(host.GetName(), "preloading image: %s", image.Filename)
			start := time.Now()

			if HasSuffixI(image.Filename, ".tar.gz", ".tgz") {
				switch kubeConf.Cluster.Kubernetes.ContainerManager {
				case "crio":
					loadCmd = "ctr" // BUG
				case "containerd":
					loadCmd = "ctr -n k8s.io images -"
				case "isula":
					loadCmd = "isula"
				default:
					loadCmd = "docker load"
				}

				if _, err := runtime.GetRunner().SudoCmd(fmt.Sprintf("env PATH=$PATH gunzip -c %s | %s", image.Filename, loadCmd), false); err != nil {
					return errors.Wrap(err, "load image failed")
				}
			} else {
				switch kubeConf.Cluster.Kubernetes.ContainerManager {
				case "crio":
					loadCmd = "ctr" // BUG
				case "containerd":
					loadCmd = "ctr -n k8s.io images"
				case "isula":
					loadCmd = "isula"
				default:
					loadCmd = "docker load -i"
				}

				if _, err := runtime.GetRunner().SudoCmd(fmt.Sprintf("env PATH=$PATH %s %s", loadCmd, image.Filename), false); err != nil {
					return errors.Wrap(err, "load image failed")
				}
			}
			logger.Log.Infof("%s load image %s success in %s", loadCmd, image.Filename, time.Since(start))
		default:
			continue
		}

	}
	return nil

}

func HasSuffixI(s string, suffixes ...string) bool {
	s = strings.ToLower(s)
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, strings.ToLower(suffix)) {
			return true
		}
	}
	return false
}
