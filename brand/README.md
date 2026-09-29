# CanDo 可为 · 品牌规范（全系列产品通用）

理念：Everyone can do research. 人人都能做科研。

## 标志
- 一个圆点 + 一笔动作：圆点是人（任何一个人），笔画是做成的事。合起来是举起双手的人，也是“完成”的勾。
- 系列产品：圆点不变，动作不同。CanDo Lab（科研台，勾）、CanDo Read（读文献，书）、CanDo Write（写论文，流线）、CanDo Agent（智能体，环）。
- 文件：`cando-icon.svg`（主图标，圆角）、`cando-icon-small.svg`（≤32 像素用，笔画加粗）、`cando-icon-fullbleed.svg`（iOS / 苹果主屏幕，系统自动切圆角）、`cando-icon-paper.svg`（浅色底）、`cando-icon-ink.svg`（深色底）、`cando-mark.svg` / `cando-mark-white.svg`（只有标志，放在文字旁边）。
- 最小尺寸 16 像素；标志四周至少留出圆点直径的空白；不要拉伸、描边、加阴影或换颜色。

## 主色系
| 名称 | 色值 | 用途 |
|---|---|---|
| 深群青（主色） | #1E2AB0 | 标志、主按钮、链接、选中 |
| 深群青·按下 | #18228F | 悬停、按下 |
| 浅群青 | #EBECFB | 选中底色、提示 |
| 墨色 | #14161F | 文字、深色背景 |
| 纸色 | #F6F4EE | 页面背景 |
| 白 | #FFFFFF | 卡片 |
| 深色模式主色 | #7C86FF | 深色背景上的按钮和链接 |
| 成功 / 提醒 / 错误 | #157A4C / #9A5B00 / #B4262B | 状态文字（深色模式另见 style.css） |

用量参考：纸 60% · 白 22% · 墨 10% · 深群青 6% · 其他 2%。白字放在深群青上对比度 10.4（远高于 4.5 的要求）。

## 命名
- 英文 CanDo，中文 可为。全称“CanDo 可为”；手机桌面等位置写“CanDo”。
- 中英文之间留一个空格；英文 C、D 大写。

## 重新生成图标
`python3 render.py`（需要 Playwright + Chromium），输出到 `png/`；再按 v1.12 的做法复制到 web/、android/res/、ios/、build/app.ico。
