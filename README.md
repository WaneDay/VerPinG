# VerPinG

独立常驻工具：通过 clash-verge-rev 的**外部控制器 API** 自动检测当前节点延时并按策略切换。
纯 Go 编写，**单文件绿色 exe**，自带系统托盘与原生设置面板，无控制台黑窗。

## 功能

- 每 N 分钟检测目标策略组「当前节点」的延时
- 当前延时 `<= 阈值`（默认 220ms）：不切换
- 当前延时 `> 阈值`：优先切到**同国家** `< 阈值` 的节点，否则切到**全组延时最低**且 `< 阈值` 的节点
- 全部节点都超阈值：切到全组当前延时最低的节点并弹窗提醒，之后继续定时巡检（`test_mode=true` 时仅记录日志，不切换不弹窗）
- 当前节点每次都重新实测延时再判定（与面板手动测速一致）；候选节点优先历史值、缺失补测，切换前再实测确认一次
- 关闭窗口仅最小化到托盘；托盘左键单击打开界面，右键可「打开界面 / 隐藏界面 / 退出」
- 仅在**系统代理开启**时工作，代理关闭时自动待机（跟随系统代理开关）
- 开机自启（写注册表 Run 键），界面内可即时启用/关闭
- 单实例运行

## 使用

1. 打开 clash-verge-rev → 设置 → 基础设置 → 外部控制器（开启），记下端口与密钥
2. 双击 `VerPinG.exe`（首次运行会在同目录生成 `config.json`）
3. 若手动启动会弹出设置面板；填好 API 地址 / 密钥 / 目标策略组后点「保存设置」
4. 右下角托盘出现图标；右键可打开/隐藏界面、退出
5. 建议先保持 `test_mode: true`，观察日志无误后再改为 `false`

开机自启或随系统代理自动拉起时默认**后台静默**（只驻留托盘，右键托盘图标可打开界面）；
也可用命令行参数 `--show` 强制启动即显示面板。

## 配置文件 `config.json`

| 键 | 说明 | 默认 |
| --- | --- | --- |
| `check_interval_minutes` | 巡检间隔（分钟） | `5` |
| `latency_threshold_ms` | 延时阈值（ms） | `220` |
| `target_group` | 目标策略组名 | `自动选择` |
| `prefer_same_country` | 是否优先同国家 | `true` |
| `controller_url` | 外部控制器地址 | `http://127.0.0.1:9090` |
| `secret` | 控制器密钥 | `set-your-secret` |
| `test_url` | 测速 URL | `https://cp.cloudflare.com/generate_204` |
| `delay_timeout_ms` | 单次测速超时（ms） | `5000` |
| `http_timeout_ms` | HTTP 请求超时（ms） | `5000` |
| `max_concurrent_delay_tests` | 并发测速数 | `4` |
| `core_ready_retries` | 内核就绪重试次数 | `7` |
| `core_ready_interval_ms` | 内核就绪重试间隔（ms） | `1800` |
| `test_mode` | 静默模式：切换照常执行，只是不弹窗 | `true` |
| `fallback_profiles` | 旧版订阅兜底，已废弃（现为最低延时切换+弹窗） | `[]` |
| `country_aliases` | 国家/地区关键词表 | 见 `config.example.json` |

配置与日志位置：若 exe 所在目录可写则就地存放（绿色版）；否则回退到
`%APPDATA%\VerPinG`。日志文件为 `<数据目录>\logs\app.log`。
安装版的实际运行目录一般是 `%LOCALAPPDATA%\Programs\VerPinG`，
面板上的「日志目录」按钮可一键打开（正在运行的实例日志以此为准）。

## 从源码构建

依赖：Go 1.27+、[rsrc](https://github.com/akavel/rsrc)（生成清单与图标资源）。

```bat
:: 国内网络建议使用代理
set GOPROXY=https://goproxy.cn,direct
go install github.com/akavel/rsrc@latest

:: 一键构建
build.bat
```

`build.bat` 会执行：

```bat
rsrc -manifest app.manifest -ico icon.ico -o rsrc.syso
go build -ldflags "-H=windowsgui -s -w" -o build\VerPinG.exe .
```

> 仓库已内置 `rsrc.syso`，若无需改动清单/图标，可直接 `go build`。

## 打包安装程序

安装 [Inno Setup](https://jrsoftware.org/isdl.php) 后，用 `installer.iss` 编译即可得到
`dist\VerPinG-Setup-*.exe`。便携版直接打包 `build\VerPinG.exe`
与 `icon.ico`、`config.example.json` 即可。

## 目录结构

```
main.go                 入口：单实例 + 日志初始化 + 拉起托盘/面板 + 后台巡检
internal/config         配置读写与默认值
internal/logger         日志（文件 + 尾部读取给界面）
internal/api            clash-verge 外部控制器 API 客户端
internal/country        节点名 -> 国家/地区 匹配（国旗 emoji / 中文 / 关键词）
internal/switcher       巡检与切换核心逻辑
internal/sysproxy       读取系统代理开关状态
internal/autostart      开机自启（注册表 Run 键）
internal/notify         系统原生提示弹窗
internal/single         单实例互斥
internal/agent          后台巡检协程（随系统代理开关启停）
internal/gui            原生窗口 + 托盘（github.com/lxn/walk）
app.manifest            程序清单（Common Controls v6 + DPI 感知）
rsrc.syso               由 rsrc 生成（已内置）
experimental/           Python 参考实现（非发布内容）
```

## 许可

[MIT](LICENSE)
