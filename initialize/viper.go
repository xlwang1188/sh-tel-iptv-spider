package initialize

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"iptv-spider-sh/config"
	"iptv-spider-sh/global"
)

// loadDotEnv loads key=value pairs from a .env file into the process environment
func loadDotEnv(filenames ...string) {
	target := ".env"
	if len(filenames) > 0 && filenames[0] != "" {
		target = filenames[0]
	}
	file, err := os.Open(target)
	if err != nil {
		return // .env is optional
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			v = strings.Trim(v, `"'`)
			// If not already explicitly set in environment, set from .env
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
	fmt.Printf("成功载入环境变量配置文件: %s\n", target)
}

// applyEnvOverrides overwrites configuration struct fields with environment variables
func applyEnvOverrides(cfg *config.Server) {
	if cfg == nil {
		return
	}
	if v := os.Getenv("SYSTEM_ADDR"); v != "" {
		cfg.System.Addr = v
	}
	if v := os.Getenv("STB_UID"); v != "" {
		cfg.Stb.UID = v
	}
	if v := os.Getenv("STB_MAC"); v != "" {
		cfg.Stb.MAC = v
	}
	if v := os.Getenv("STB_SN"); v != "" {
		cfg.Stb.SN = v
	}
	if v := os.Getenv("STB_IP"); v != "" {
		cfg.Stb.IP = v
	}
	if v := os.Getenv("STB_TYPE"); v != "" {
		cfg.Stb.Type = v
	}
	if v := os.Getenv("STB_AUTH_HOST"); v != "" {
		cfg.Stb.AuthHost = v
	}
	if v := os.Getenv("EPG_XML_URL"); v != "" {
		cfg.Epg.XmlUrl = v
	}
	if v := os.Getenv("EPG_RTSP_URL"); v != "" {
		cfg.Epg.RtspUrl = v
	}
	if v := os.Getenv("EPG_RTP_URL"); v != "" {
		cfg.Epg.RtpUrl = v
	}
	if v := os.Getenv("EPG_LOGO_URL"); v != "" {
		cfg.Epg.LogoUrl = v
	}
	if v := os.Getenv("EPG_FETCH_CRON"); v != "" {
		cfg.Epg.FetchCron = v
	}
}

func Viper(path ...string) *viper.Viper {
	var configFile string
	if len(path) == 0 {
		flag.StringVar(&configFile, "c", "", "choose config file.")
		flag.Parse()
		if configFile == "" { // 优先级: 命令行 > 环境变量 > 默认值
			if configEnv := os.Getenv("GO_CONFIG"); configEnv == "" {
				if _, err := os.Stat("config.yaml"); err == nil {
					configFile = "config.yaml"
				} else if _, err := os.Stat("config.example.yaml"); err == nil {
					configFile = "config.example.yaml"
				} else {
					configFile = "config.yaml"
				}
				fmt.Printf("您正在使用config的默认值,config的路径为%v\n", configFile)
			} else {
				configFile = configEnv
				fmt.Printf("您正在使用GO_CONFIG环境变量,config的路径为%v\n", configFile)
			}
		} else {
			fmt.Printf("您正在使用命令行的-c参数传递的值,config的路径为%v\n", configFile)
		}
	} else {
		configFile = path[0]
		fmt.Printf("您正在使用func Viper()传递的值,config的路径为%v\n", configFile)
	}

	// 优先加载 .env 文件中的环境变量
	loadDotEnv(os.Getenv("ENV_FILE"))

	v := viper.New()
	v.SetConfigFile(configFile)
	v.SetConfigType("yaml")
	err := v.ReadInConfig()
	if err != nil {
		fmt.Printf("Warning: 读取配置文件 %s 异常: %s (将尝试通过环境变量补全配置)\n", configFile, err)
	}
	v.WatchConfig()

	v.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("config file changed:", e.Name)
		if err := v.Unmarshal(&global.CONFIG); err != nil {
			fmt.Println(err)
		}
		applyEnvOverrides(global.CONFIG)
	})

	if err := v.Unmarshal(&global.CONFIG); err != nil {
		fmt.Println("Unmarshal error:", err)
	}
	applyEnvOverrides(global.CONFIG)

	return v
}
