# Third-party notices

本项目原创代码按根目录 `LICENSE` 中的 MIT 许可证授权。发布的可执行文件还包含下列第三方组件；这些组件仍受各自许可证约束。

## MuPDF

- 版本：随 go-fitz `v1.28.2-cjk.4` 捆绑；捆绑头文件 `FZ_VERSION` 为 `1.28.0`
- 许可证：GNU Affero General Public License v3.0（AGPL-3.0）
- 项目主页：<https://mupdf.com/>
- 许可证说明：<https://mupdf.com/license.html>
- 完整文本：`LICENSES/AGPL-3.0.txt`

`pdfreader` 的发布二进制静态链接了 MuPDF。因此，**发布二进制整体作为 AGPL-3.0 组合作品分发**；本仓库即为对应源码（Corresponding Source）。如果你需要以其他许可证分发二进制，需要自行替换 PDF 引擎并遵守其许可证。

## go-fitz

- 项目：`github.com/qiangmzsx/go-fitz`（MuPDF 的 Go 封装，CJK 分支）
- 版本：v1.28.2-cjk.4
- 许可证：AGPL-3.0（见上游仓库 `COPYING`）
- 仓库：<https://github.com/qiangmzsx/go-fitz>

## Gio

- 项目：`gioui.org`
- 版本：v0.10.3
- 许可证：Unlicense OR MIT
- 仓库：<https://git.sr.ht/~eliasnaur/gio>
- 许可证文本：`LICENSES/GIO.txt`

## Noto Sans SC

- 文件：`assets/NotoSansSC-Regular.ttf`
- 版权：Copyright 2014-2021 Adobe (http://www.adobe.com/), with Reserved Font Name 'Source'
- 许可证：SIL Open Font License 1.1（OFL-1.1）
- 完整文本：`LICENSES/OFL-1.1.txt`

该字体通过 `go:embed` 嵌入程序，仅用于界面文字；PDF 正文中文由 MuPDF 调用系统中文字体渲染。

## Go 标准库与 golang.org/x

Go 标准库及 `golang.org/x/*` 依赖按 BSD 3-Clause 许可证分发。各组件的完整声明以其源码仓库中的 LICENSE 文件为准。
