package fastbuilder

import (
	"bufio"
	"fmt"
	"io"
	"omega_launcher/cqhttp"
	"omega_launcher/utils"
	"os/exec"
	"path"
	"time"

	"github.com/pterm/pterm"
)

// 启动器配置文件结构
type BotConfig struct {
	RentalCode       string `json:"租赁服号"`
	RentalPasswd     string `json:"租赁服密码"`
	FBToken          string `json:"FBToken"`
	QGroupLinkEnable bool   `json:"是否开启群服互通"`
	StartOmega       bool   `json:"是否启动Omega"`
	UpdateFB         bool   `json:"是否更新FB"`
}

// 配置Token
func FBTokenSetup(cfg *BotConfig) *BotConfig {
	if cfg.FBToken != "" {
		pterm.Info.Printf("要使用上次的 Fastbuilder 账号登录吗?  要请输入 y , 需要修改请输入 n: ")
		if utils.GetInputYN() {
			return cfg
		}
	}
	cfg.FBToken = RequestToken()
	return cfg
}

// 配置租赁服信息
func RentalServerSetup(cfg *BotConfig) *BotConfig {
	pterm.Info.Printf("请输入租赁服账号: ")
	cfg.RentalCode = utils.GetValidInput()
	pterm.Info.Printf("请输入租赁服密码（没有则留空）: ")
	cfg.RentalPasswd = utils.GetInput()
	return cfg
}

func StartHelper() {
	// 读取配置出错则退出
	botConfig := &BotConfig{}
	if err := utils.GetJsonData(path.Join(utils.GetCurrentDir(), "服务器登录配置.json"), botConfig); err != nil {
		panic(err)
	}
	// 询问是否使用上一次的配置
	if botConfig.FBToken != "" && botConfig.RentalCode != "" {
		pterm.Info.Printf("要使用和上次完全相同的配置启动吗? 要请输入 y, 不要请输入 n: ")
		if utils.GetInputYN() {
			// 更新FB
			if botConfig.UpdateFB {
				UpdateFB()
			}
			// 群服互通
			if botConfig.QGroupLinkEnable {
				if utils.IsDir(path.Join(utils.GetCurrentDir(), "omega_storage")) {
					cqhttp.RunCQHttp()
				} else {
					pterm.Warning.Println("在Omega完全启动前，将不会进行群服互通的配置")
				}
			}
			Run(botConfig)
			return
		}
	}
	// 配置FB更新
	pterm.Info.Printf("需要从官网下载或更新 Fastbuilder 吗?  要请输入 y, 不要请输入 n: ")
	if utils.GetInputYN() {
		UpdateFB()
		botConfig.UpdateFB = true
	} else {
		pterm.Warning.Println("将会使用该路径的 Fastbuilder：" + GetFBExecPath())
		botConfig.UpdateFB = false
		time.Sleep(time.Second)
	}
	// 配置FB
	botConfig = FBTokenSetup(botConfig)
	// 配置租赁服登录
	if botConfig.RentalCode != "" {
		pterm.Info.Printf("要使用上次的租赁服配置吗?  要请输入 y, 不要请输入 n : ")
		if !utils.GetInputYN() {
			botConfig = RentalServerSetup(botConfig)
		}
	} else {
		botConfig = RentalServerSetup(botConfig)
	}
	// 询问是否使用Omega
	pterm.Info.Printf("要启动 Omega 还是 Fastbuilder?  启动 Omega 请输入 y, 启动 Fastbuilder 请输入 n: ")
	if utils.GetInputYN() {
		botConfig.StartOmega = true
		// 配置群服互通
		if utils.IsDir(path.Join(utils.GetCurrentDir(), "omega_storage")) {
			pterm.Info.Printf("要启用群服互通吗?  要请输入 y, 不要请输入 n: ")
			if utils.GetInputYN() {
				cqhttp.CQHttpEnablerHelper()
				botConfig.QGroupLinkEnable = true
			} else {
				botConfig.QGroupLinkEnable = false
			}
		} else {
			pterm.Warning.Println("在Omega完全启动前，将不会进行群服互通的配置")
			botConfig.QGroupLinkEnable = false
		}
	}
	// 将本次配置写入文件
	if err := utils.WriteJsonData(path.Join(utils.GetCurrentDir(), "服务器登录配置.json"), botConfig); err != nil {
		pterm.Error.Println("无法记录配置，不过可能不是什么大问题")
	}
	// 启动Omega或者FB
	Run(botConfig)
}

func Run(cfg *BotConfig) {
	// 配置启动参数
	args := []string{"-M", "--plain-token", cfg.FBToken, "--no-update-check", "-c", cfg.RentalCode}
	// 是否需要租赁服密码
	if cfg.RentalPasswd != "" {
		args = append(args, "-p")
		args = append(args, cfg.RentalPasswd)
	}
	// 是否启动Omega
	if cfg.StartOmega {
		args = append(args, "-O")
	}
	// 建立频道
	readC := make(chan string)
	stop := make(chan string)
	// 持续将输入信息输入到频道中
	go func() {
		for {
			s := utils.GetInput()
			readC <- s
		}
	}()
	for {
		// 启动时提示信息
		pterm.Success.Println("如果 Omega/Fastbuilder 崩溃了，它会在最长 10 秒后自动重启")
		// 启动命令
		cmd := exec.Command(GetFBExecPath(), args...)
		// 建立从Fastbuilder到控制台的输出管道
		omega_out, err := cmd.StdoutPipe()
		if err != nil {
			panic(err)
		}
		// 建立从控制台到Fastbuilder的输入管道
		omega_in, err := cmd.StdinPipe()
		if err != nil {
			panic(err)
		}
		// 从管道中获取并打印Fastbuilder输出内容
		go func() {
			reader := bufio.NewReader(omega_out)
			for {
				readString, err := reader.ReadString('\n')
				if err != nil || err == io.EOF {
					pterm.Error.Println("读取Fastbuilder输出内容时出现错误")
					return
				}
				fmt.Print(readString)
			}
		}()
		// 在未收到停止信号前，启动器会一直将控制台输入的内容通过管道发送给Fastbuilder
		go func() {
			for {
				select {
				case <-stop:
					return
				case s := <-readC:
					omega_in.Write([]byte(s + "\n"))
				}
			}
		}()
		// 启动并持续运行Fastbuilder
		err = cmd.Start()
		if err != nil {
			pterm.Error.Println("Fastbuilder启动时出现错误")
		}
		err = cmd.Wait()
		if err != nil {
			pterm.Error.Println("Fastbuilder运行时出现错误")
		}
		// 如果运行到这里，说明Fastbuilder出现错误了
		// 随便往频道丢点东西结束协程
		stop <- "stop!!"
		pterm.Error.Println("Oh no! Fastbuilder crashed!") // ?
		// 为了避免频繁请求，崩溃后将等待10秒后重启，可手动跳过等待
		pterm.Warning.Print("似乎发生了错误，要重启 Fastbuilder 吗? 请按回车确认(10秒后会自动确认): ")
		// 等待输入或计时结束
		select {
		case <-readC:
		case <-time.After(time.Second * 10):
			fmt.Println("")
		}
	}
}