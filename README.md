# TextViewer

使用 Go / MyGo 开发的原生文本查看编辑器，面向 Windows 11。

## 功能

- 打开、拖入、编辑和保存文本；支持撤销、重做、自动换行和字号调整。
- 异步查找、上一处/下一处、替换和全部替换；支持区分大小写，替换可撤销。
- 按文件名识别 JSON、XML、Go、Python、JavaScript、TypeScript、HTML、CSS、SQL 等代码格式；右上角可手动选择语言或纯文本。
- 代码使用等宽字体、语法着色、缩进参考线和括号匹配；只改变显示，不自动重排文件内容。
- TXT、LOG 和未知扩展名默认纯文本，不为猜测格式扫描全文。打开新文件会恢复自动识别；手动选择仅用于当前文件。
- UTF-8、UTF-8 BOM、UTF-16 LE/BE BOM 和 GB18030；保存保留编码及 CRLF/LF 换行方式。
- 浅色、深色和跟随系统；上排 SVG 按钮包含悬浮提示和辅助功能名称。
- Windows 11 新版资源管理器右键命令“在 TextViewer 中打开”，不是“显示更多选项”里的旧式注册表命令。
- 在线检查正式版本更新；下载安装前确认，下载后校验 SHA-256，安装前提示保存未保存的文本。

文件被外部程序修改、编码有损或文件只读时，不会直接覆盖原文件。混合 CRLF/LF 的文件由编辑器统一换行方式，不逐行保留混合换行。

## 使用

从 [v0.0.1 发布页](https://github.com/Oranjekop/TextViewer/releases/tag/v0.0.1) 下载 `TextViewer-Setup-0.0.1-windows.exe`。同一个安装程序自动选择 x64 / ARM64，按当前用户安装，无需为主程序申请管理员权限，提供开始菜单入口、可选桌面快捷方式及系统“已安装的应用”中的卸载入口。

需要便携使用时，下载对应架构的 ZIP，并完整解压。安装目录或解压目录中的 `TextViewer.exe` 支持拖入文件，也可把文件路径作为启动参数。启动新实例时，文件交给现有窗口打开。

安装程序目前没有公共 Authenticode 签名；不要把右键 MSIX 的自签名当作安装程序获得 Windows 公共发行商认证。

| 操作 | 快捷键 |
| --- | --- |
| 新建 / 打开 / 保存 | Ctrl+N / Ctrl+O / Ctrl+S |
| 另存为 | Ctrl+Shift+S |
| 查找 / 替换 | Ctrl+F / Ctrl+H |
| 下一处 / 上一处 | F3 / Shift+F3 |
| 关闭查找栏 | Esc |
| 自动换行 | Alt+Z |
| 增大 / 缩小字号 | Ctrl+加号 / Ctrl+减号 |
| 恢复 16 号字 | Ctrl+0 |
| 撤销 / 重做 | Ctrl+Z / Ctrl+Y |

### Windows 11 新版右键

完整构建目录包含程序、原生 COM 扩展、带签名的稀疏 MSIX 身份包及配套脚本。菜单命令接受一个普通文件，不对文件夹或多选显示。

1. 从“设置”选择“添加到 Windows 11 新版右键菜单”。
2. 本机开发签名尚未受信任时，应用先询问是否继续；是否接受随后出现的 Windows 管理员确认由用户决定。
3. 在资源管理器中右键单个文本文件，选择“在 TextViewer 中打开”。
4. 从“设置”选择“移除右键菜单”可以撤销注册。

本机签名信任仅导入指定的终端签名证书至 `LocalMachine\TrustedPeople`，不会导入根证书、开启开发者模式或更改 TXT 默认打开方式。开发证书私钥不会随包分发。正式分发可以在构建时指定已有代码签名证书。移除菜单不会删除系统中的证书信任。

注册时完整程序复制到 `%LOCALAPPDATA%\Programs\TextViewer\app-<内容摘要>`，避免便携版目录被移动后菜单失效。

安装包只尝试注册已经受信任的右键身份包，不会自动导入证书。CI 未配置持久发行签名证书时，每次构建会生成新的自签名，更新后启用右键可能需要再次确认信任。

当前验证边界：原生 COM 命令测试通过；本机尚未批准证书信任，因此还未完成 Windows 11 26H2 首层菜单安装及目视验收。不要把 DLL 测试或 MSIX 签名成功当成实际安装成功。

### 在线更新

- 默认启动 3 秒后在后台检查，24 小时内最多自动检查一次；检查失败也会限频，不影响文本编辑。
- 从“设置 → 检查更新”随时手动检查，也可关闭“启动时检查更新”。仅检查 GitHub 上本仓库的正式 Release，不提供预发布或降级。
- 只有确认后才下载安装包。下载流式校验 SHA-256 清单及 GitHub 提供的摘要，失败或取消会删除未完成文件；执行前再次校验文件。
- 安装前提示保存修改，然后打开交互式安装程序。安装程序不会强制关闭仍在运行的 TextViewer；更新便携版时会安装到常规安装目录。
- 检查超时为 15 秒，元数据上限 2 MiB；安装包上限 256 MiB，下载进度最多每 200 ms 更新一次，不扫描或复制正在编辑的全文。

## 大文件与性能

文本存放于分块、持久化的编辑缓冲区，编辑、查找和保存不依赖每次复制全文的 UI 字符串。文件载入后仍驻留内存，不是无限大小的磁盘分页编辑器。

输入保护上限为 2 GiB；这不等于已验证任意 2 GiB 文件。已实测约 100 MiB 的多行 TXT 和 JSON。单行超过 10,000 UTF-8 字节时跳过着色并重置后续词法状态，避免超长压缩代码的高亮开销；仍保留原文。极长单行的布局成本不在多行样本的保证范围内。

2026-11-02，本机 CPU 离屏渲染测试：

| 项目 | TXT，100 MiB / 819,201 行 | JSON，约 100 MiB / 1,028,100 行 |
| --- | ---: | ---: |
| 打开 | 586 ms | 506 ms |
| 载入后新增保留堆，不含后续 UI 索引 | 108.8 MiB | 105.0 MiB |
| 稳态重绘 | 0.50 ms | 0.39 ms |
| 输入一个字符 | 6.0 ms | 7.5 ms |
| 全文无匹配查找 | 1.35 s | 1.26 s |
| 保存 | 671 ms | 889 ms |

TXT 自动换行索引在初始测试批次后渐进完成，另用约 1.41 s。JSON 跳到文件末尾时，头一批离屏帧约 186 ms，后续渐进词法状态追赶约 2.42 s；不在切换语言时一次性预解析全文。报告中的 `firstFrameMS` 和 `navigateEndMS` 包含测试器最多 20 帧的批次，不是单帧延迟或 GPU 指标。上述耗时受硬件、文件内容和当前系统负载影响。

复测会生成临时文件，不读取或覆盖现有用户文档：

```powershell
go run . --performance-check -size-mb 100 -output dist/evidence
go run . --performance-check -size-mb 100 -format json -output dist/evidence
```

## 开发与构建

依赖 Go 1.27.3、MyGo v0.7.3。首次构建可下载经固定 SHA-256 校验的 LLVM-MinGW 和 Windows SDK 工具：

```powershell
go test ./...
go vet ./...
go mod verify
.\scripts\build.ps1 -BootstrapTools
.\scripts\test-shell.ps1
```

输出目录为 `dist\TextViewer\windows-amd64`，包含 `TextViewer.exe`、安装程序及右键配套文件。

```powershell
# 使用新的输出目录，避免覆盖正在运行的构建
.\scripts\build.ps1 -OutputDirectory dist\TextViewer-current

# 构建 ARM64
.\scripts\build.ps1 -Architecture arm64

# 使用明确选择的现有代码签名证书
.\scripts\build.ps1 -CertificateThumbprint <证书指纹>

# 生成浅色/深色、760/1120 宽度的文本及代码预览
$env:TEXTVIEWER_PREVIEW_DIR = "$PWD\dist\evidence\native-previews"
go test ./internal/desktop -run TestPreview -count=1
```

MyGo 构建会临时生成 `.syso` 资源文件，同一目录内不要并行执行构建、测试或 vet。开发中运行 GUI 可使用 `go run .`；右键注册必须使用包含完整配套文件的构建目录。

### 发行安装包

```powershell
.\scripts\build-release.ps1 -Tag v0.0.1 -BootstrapTools

# 在独立临时目录实测原生架构选择、版本和安装/卸载；不注册真实右键或更改证书信任
$env:TEXTVIEWER_TEST_INSTALLER = '1'
go test ./cmd/release -run '^TestUniversalInstallerInstallsNativePayload$' -count=1 -v -timeout 180s
Remove-Item Env:\TEXTVIEWER_TEST_INSTALLER
```

输出到 `dist\release\v0.0.1`：

| 文件 | 用途 |
| --- | --- |
| `TextViewer-Setup-0.0.1-windows.exe` | x64 / ARM64 合并安装包 |
| `TextViewer-0.0.1-windows-amd64.zip` | x64 完整便携包 |
| `TextViewer-0.0.1-windows-arm64.zip` | ARM64 完整便携包 |
| `SHA256SUMS.txt` | 三个公开产物的 SHA-256 |

`.github/workflows/release.yml` 对 main / PR 执行测试、vet 及依赖验证；推送 `vMAJOR.MINOR.PATCH` 标签时，从标签写入应用版本、EXE 版本和 MSIX 身份版本，再顺序编译双架构、运行原生右键测试与隔离安装验收，最后校验完整产物并发布正式 Release。不会覆盖已经正式发布的版本。

后续发布先添加 `docs/releases/<版本>.md`，提交代码后创建并推送对应标签。每段版本号必须是无前导零的整数且不超过 65535，以匹配 Windows 安装包版本限制。

发布后可使用真实更新客户端检查公开版本并下载校验安装包，不执行安装：

```powershell
$env:TEXTVIEWER_UPDATE_LIVE_CHECK = '1'
$env:TEXTVIEWER_EXPECTED_RELEASE = '0.0.1'
go test ./internal/update -run '^TestGitHubReleaseUpdateIntegration$' -count=1 -v -timeout 240s
Remove-Item Env:\TEXTVIEWER_UPDATE_LIVE_CHECK, Env:\TEXTVIEWER_EXPECTED_RELEASE
```

### 图标资产

- 工具栏 SVG 位于 `internal/desktop/icons`，采用统一 24×24 网格与线宽；仅在启动时解析一次，随主题着色。
- `assets/app-icon-master.png` 是内置生图工具生成的原始母版，最终提示词及来源记录在 `assets/app-icon-prompt.md`。
- `go run ./cmd/assets assets` 从母版生成 EXE 与 MSIX 所需 PNG，保留透明度，不会用占位图覆盖生图设计。
