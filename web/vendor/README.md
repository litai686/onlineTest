# 第三方前端资源

均为静态文件，随 `go:embed` 打入二进制，无构建步骤、无外网依赖。

## KaTeX 0.19.1

- 文件：`katex.mjs`、`katex.min.css`、`fonts/`
- 用途：题目、选项、答案、解析的公式渲染（`trust:false`、`throwOnError:false`）
- 许可：MIT（见 `LICENSE`）
- 来源：https://www.npmjs.com/package/katex

## MathLive 0.111.1

- 文件：`mathlive.min.mjs`、`mathlive-fonts.css`、`fonts/`、`sounds/`
- 用途：公式编辑浮层（虚拟键盘、手写输入可选），输出 LaTeX
- 许可：MIT（见 `LICENSE.txt`）
- 来源：https://www.npmjs.com/package/mathlive

升级方式：下载对应 npm 包，替换上述文件与字体目录，更新本文件中的版本号，然后重新 `go build`。
