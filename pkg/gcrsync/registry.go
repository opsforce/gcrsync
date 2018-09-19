/*
 * Copyright © 2018 mritd <mritd1234@gmail.com>
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
 * THE SOFTWARE.
 */

package gcrsync

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/json-iterator/go"

	"github.com/Sirupsen/logrus"

	"github.com/opsforce/gcrsync/pkg/utils"
)

func (g *Gcr) needProcessImages(images []string) []string {
	var needSyncImages []string
	var imgGetWg sync.WaitGroup
	imgGetWg.Add(len(images))
	imgNameCh := make(chan string, 20)

	for _, imageName := range images {
		tmpImageName := imageName
		go func() {
			defer func() {
				g.QueryLimit <- 1
				imgGetWg.Done()
			}()

			select {
			case <-g.QueryLimit:
				if !g.queryRegistryImage(tmpImageName) {
					imgNameCh <- tmpImageName
				}
			}
		}()
	}

	var imgReceiveWg sync.WaitGroup
	imgReceiveWg.Add(1)
	go func() {
		defer imgReceiveWg.Done()
		for {
			select {
			case imageName, ok := <-imgNameCh:
				if ok {
					needSyncImages = append(needSyncImages, imageName)
				} else {
					goto imgSetExit
				}
			}
		}
	imgSetExit:
	}()

	imgGetWg.Wait()
	close(imgNameCh)
	imgReceiveWg.Wait()
	return needSyncImages

}

func (g *Gcr) queryRegistryImage(imageName string) bool {
	imageInfo := strings.Split(imageName, ":")
	addr := fmt.Sprintf(RegistryTag, g.DockerUser, imageInfo[0], imageInfo[1])
	req, _ := http.NewRequest("GET", addr, nil)
	resp, err := g.httpClient.Do(req)
	if !utils.CheckErr(err) {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		logrus.Debugf("Image [%s] found, skip!", imageName)
		return true
	} else {
		return false
	}
}

func (g *Gcr) compareCache(images []string) []string {
	var cachedImages []string
	repoDir := strings.Split(g.GithubRepo, "/")[1]
	f, err := os.Open(filepath.Join(repoDir, g.NameSpace))
	utils.CheckAndExit(err)
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	utils.CheckAndExit(err)
	jsoniter.Unmarshal(b, &cachedImages)
	logrus.Infof("Cached images total: %d", len(cachedImages))

	return utils.SliceDiff(images, cachedImages)
}
