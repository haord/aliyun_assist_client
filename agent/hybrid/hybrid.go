package hybrid

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/aliyun/aliyun_assist_client/agent/log"
	"github.com/aliyun/aliyun_assist_client/agent/metrics"
	"github.com/aliyun/aliyun_assist_client/agent/util"
	"github.com/aliyun/aliyun_assist_client/agent/util/osutil"
	"github.com/aliyun/aliyun_assist_client/agent/util/process"
	"github.com/aliyun/aliyun_assist_client/agent/version"
	"github.com/tidwall/gjson"
)

type RegisterInfo struct {
	Code            string `json:"activationCode"`
	MachineId       string `json:"machineId"`
	RegionId        string `json:"regionId"`
	InstanceName    string `json:"instanceName"`
	Hostname        string `json:"hostname"`
	IntranetIp      string `json:"intranetIp"`
	OsVersion       string `json:"osVersion"`
	OsType          string `json:"osType"`
	ClientVersion   string `json:"agentVersion"`
	PublicKeyBase64 string `json:"publicKey"`
	Id              string `json:"activationId"`
}

type registerResponse struct {
	Code       int    `json:"code"`
	InstanceId string `json:"instanceId"`
}

type unregisterResponse struct {
	Code int `json:"code"`
}

func Register(region string, code string, id string, name string, networkmode string, need_restart bool) (ret bool) {
	log.GetLogger().Infoln(region, code, id, name)
	errmsg := ""
	defer func() {
		msgkey := "info"
		status := "success"
		if !ret {
			msgkey = "errmsg"
			status = "failed"
		}
		metrics.GetHybridRegisterEvent(
			ret,
			"status", status,
			msgkey, errmsg,
		).ReportEvent()
	}()

	ret = true
	if util.IsHybrid() {
		fmt.Println("error, agent already register, deregister first")
		errmsg = "error, agent already register, deregister first"
		log.GetLogger().Infoln("error, agent already register, deregister first")
		return false
	}
	hostname, _ := os.Hostname()
	osType := "unknown"
	if runtime.GOOS == "windows" {
		osType = "windows"
	} else if runtime.GOOS == "linux" {
		osType = "linux"
	}
	ip, _ := osutil.ExternalIP()
	var pub, pri bytes.Buffer
	err := genRsaKey(&pub, &pri)
	if err != nil {

		errmsg = fmt.Sprintf("generate rsa key error: %s", err.Error())
		fmt.Println("error, generate rsa key failed")
		return false
	}
	encodeString := base64.StdEncoding.EncodeToString(pub.Bytes())
	mid, _ := util.GetMachineID()
	info := &RegisterInfo{
		Code:            code,
		MachineId:       mid,
		RegionId:        region,
		InstanceName:    name,
		Hostname:        hostname,
		IntranetIp:      ip.String(),
		OsVersion:       osutil.GetVersion(),
		OsType:          osType,
		ClientVersion:   version.AssistVersion,
		PublicKeyBase64: encodeString,
		Id:              id,
	}
	jsonBytes, _ := json.Marshal(*info)
 	var response string
	domain := util.HYBRID_DOMAIN
	if networkmode == "vpc" {
		domain = util.HYBRID_DOMAIN_VPC
	}
	url := "https://" + region + domain + "/luban/api/instance/register"	
	response, err = util.HttpPost(url, string(jsonBytes), "")
	if err != nil {
		ret = false
		errmsg = fmt.Sprintf("register request err: %s, url=%s", err.Error(), url)
		fmt.Println(response, err)
		return
	}

	if !gjson.Valid(response) {
		ret = false
		errmsg = fmt.Sprintf("invalid task info json, url=%s, response=%s", url, response)
		fmt.Println("invalid task info json:", response)
		return
	}

	var register_response registerResponse
	if err := json.Unmarshal([]byte(response), &register_response); err == nil {
		if register_response.Code == 200 {
			var path string
			if util.IsSelfHosted() {
				path, _ = util.GetSelfhostedPath()
			} else {
				path, _ = util.GetHybridPath()
			}
			util.WriteStringToFile(path+"/network-mode", networkmode)
			util.WriteStringToFile(path+"/pub-key", pub.String())
			util.WriteStringToFile(path+"/pri-key", pri.String())
			util.WriteStringToFile(path+"/region-id", region)
			util.WriteStringToFile(path+"/instance-id", register_response.InstanceId)
			util.WriteStringToFile(path+"/machine-id", mid)
		} else {
			ret = false
		}
	}

	if !ret {
		errmsg = fmt.Sprintf("register failed, responsecode=%d, url=%s", register_response.Code, url)
		fmt.Println("register failed")
		fmt.Println(response)
		return
	} else {
		errmsg = fmt.Sprintf("register ok, instanceid=%s", register_response.InstanceId)
		fmt.Println("register ok")
		fmt.Println("instance id:", register_response.InstanceId)
		if need_restart {
			restartService()
		}
		fmt.Println("restart service")
	}
	return
}

func UnRegister(need_restart bool) bool {
	errmsg := ""
	defer func() {
		if len(errmsg) > 0 {
			metrics.GetHybridUnregisterEvent(
				false,
				"status", "failed",
				"errormsg", errmsg,
			).ReportEvent()
		} else {
			metrics.GetHybridUnregisterEvent(
				true,
				"status", "success",
			).ReportEvent()
		}
	}()

	url := "https://" + util.GetServerHost()
	url += "/luban/api/instance/deregister"

	response, err := util.HttpPost(url, "", "")
	if err != nil {
		errmsg = fmt.Sprintf("deregister request err: %s", err.Error())
		fmt.Println(response)
	}
	ret := true
	var unregister_response unregisterResponse
	if err := json.Unmarshal([]byte(response), &unregister_response); err == nil {
		if unregister_response.Code == 200 {
			ret = true
		} else {
			ret = false
		}
	}

	if !ret {
		errmsg = fmt.Sprintf("unregister failed, responsecode=%d", unregister_response.Code)
		fmt.Println("unregister failed")
		fmt.Println(response)
	} else {
		fmt.Println("unregister ok")
		clean_unregister_data(need_restart)
	}
	return ret
}

//RSA公钥私钥产生
func genRsaKey(pub io.Writer, pri io.Writer) error {
	// 生成私钥文件
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	derStream := x509.MarshalPKCS1PrivateKey(privateKey)
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derStream,
	}

	err = pem.Encode(pri, block)
	if err != nil {
		return err
	}
	// 生成公钥文件
	publicKey := &privateKey.PublicKey
	derPkix, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return err
	}
	block = &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: derPkix,
	}

	err = pem.Encode(pub, block)
	if err != nil {
		return err
	}
	return nil
}

func restartService() {
	processer := process.ProcessCmd{}
	if runtime.GOOS == "linux" {
		processer.SyncRunSimple("aliyun-service", strings.Split("--stop", " "), 10)
		processer.SyncRunSimple("aliyun-service", strings.Split("--start", " "), 10)
	} else if runtime.GOOS == "windows" {
		processer.SyncRunSimple("net", strings.Split("stop AliyunService", " "), 10)
		processer.SyncRunSimple("net", strings.Split("start AliyunService", " "), 10)
	}
}

func clean_unregister_data(need_restart bool) {
	path, _ := util.GetHybridPath()
	os.Remove(path + "/pub-key")
	os.Remove(path + "/pri-key")
	os.Remove(path + "/region-id")
	os.Remove(path + "/instance-id")
	os.Remove(path + "/machine-id")

	if need_restart {
		restartService()
		fmt.Println("restart service")
	}
}
