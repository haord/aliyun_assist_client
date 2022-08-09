// Copyright (c) 2009-present, Alibaba Cloud All rights reserved.
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
package main

import (
	"fmt"
	"os"

	"github.com/aliyun/aliyun_assist_client/agent/pluginmanager/acspluginmanager/thirdparty/aliyun-cli/cli"
	"github.com/aliyun/aliyun_assist_client/agent/pluginmanager/acspluginmanager/thirdparty/aliyun-cli/i18n"
	"github.com/aliyun/aliyun_assist_client/agent/log"
	pm "github.com/aliyun/aliyun_assist_client/agent/pluginmanager/acspluginmanager"
	"github.com/aliyun/aliyun_assist_client/agent/pluginmanager/acspluginmanager/flag"
	"github.com/aliyun/aliyun_assist_client/agent/util"
	"github.com/aliyun/aliyun_assist_client/thirdparty/single"
)

var SingleAppLock *single.Single

var (
	gitHash   string
	assistVer string = "10.10.10.10000"
)

func main() {

	cli.Version = assistVer
	log.InitLog("acs_plugin_manager.log", "")
	cli.PlatformCompatible()
	writer := cli.DefaultWriter()

	i18n.SetLanguage("en")

	// create root command
	rootCmd := &cli.Command{
		Name:              "acs-plugin-manager",
		Short:             i18n.T("Alibaba Cloud Assist Plugin Manager Line Interface Version "+cli.Version, "阿里云云助手插件管理命令行工具 "+cli.Version),
		Usage:             "acs-plugin-manager [Flags]",
		Sample:            "",
		EnableUnknownFlag: true,
		Run:               execute,
	}

	// add default flags
	flag.AddFlags(rootCmd.Flags())

	ctx := cli.NewCommandContext(writer)
	ctx.EnterCommand(rootCmd)
	ctx.SetCompletion(cli.ParseCompletionForShell())
	rootCmd.Execute(ctx, os.Args[1:])
}

func execute(ctx *cli.Context, args []string) error {
	verbose := flag.VerboseFlag(ctx.Flags()).IsAssigned()
	pluginManager, err := pm.NewPluginManager(verbose)
	if err != nil {
		return err
	}

	version := flag.VersionFlag(ctx.Flags()).IsAssigned()
	list := flag.ListFlag(ctx.Flags()).IsAssigned()
	local := flag.LocalFlag(ctx.Flags()).IsAssigned()
	verify := flag.VerifyFlag(ctx.Flags()).IsAssigned()
	status := flag.StatusFlag(ctx.Flags()).IsAssigned()
	exec := flag.ExecFlag(ctx.Flags()).IsAssigned()

	plugin, _ := flag.PluginFlag(ctx.Flags()).GetValue()
	pluginId, _ := flag.PluginIdFlag(ctx.Flags()).GetValue()
	pluginVersion, _ := flag.PluginVersionFlag(ctx.Flags()).GetValue()
	params, _ := flag.ParamsFlag(ctx.Flags()).GetValue()
	paramsV2, _ := flag.ParamsV2Flag(ctx.Flags()).GetValue()
	url, _ := flag.UrlFlag(ctx.Flags()).GetValue()
	separator, _ := flag.SeparatorFlag(ctx.Flags()).GetValue()
	file, _ := flag.FileFlag(ctx.Flags()).GetValue()

	if verbose {
		log.GetLogger().Infof("verbose[%v]  list[%v]  local[%v]  verify[%v]  status[%v]  exec[%v]  plugin[%v]  pluginId[%v]  pluginversion[%v]  params[%v]  paramsV2[%s]  url[%v]  separator[%v]  file[%v]  ",
			verbose, list, local, verify, status, exec, plugin, pluginId, pluginVersion, params, paramsV2, url, separator, file)
	}

	if plugin != "" {
		SingleAppLock = single.New(plugin)
		if err := SingleAppLock.CheckLock(); err != nil && err == single.ErrAlreadyRunning {
			fmt.Println("exit by another plugin process is running")
			log.GetLogger().Infoln("exit by another plugin process is running")
			return nil
		}
	}

	exitCode := 0
	if version {
		fmt.Println(assistVer)
	} else if list {
		err = pluginManager.List(plugin, local)
	} else if verify {
		exitCode, err = pluginManager.VerifyPlugin(url, params, separator, paramsV2)
	} else if status {
		err = pluginManager.ShowPluginStatus()
	} else if exec {
		exitCode, err = pluginManager.ExecutePlugin(file, plugin, pluginId, params, separator, paramsV2, pluginVersion, local)
	} else {
		ctx.Command().PrintFlags(ctx)
	}
	if err != nil {
		log.GetLogger().Errorln(err)
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	if err != nil {
		fmt.Println(err)
	}

	return nil
}

func checkEndpoint() {
	if hostServer := util.GetServerHost(); hostServer == "" {
		fmt.Print("CheckEndPoint " + pm.CHECK_ENDPOINT_FAIL_STR + "Could not find a endpoint to connect server.")
		os.Exit(pm.CHECK_ENDPOINT_FAIL)
	}
}