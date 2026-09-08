# 枫讯斗地主网页版

在 [palemoky/fight-the-landlord](https://github.com/palemoky/fight-the-landlord) 的 Go 规则与 DouZero 推理基础上开发，保留 GPL-3.0 许可证和原作者署名。原终端版本的说明仍在 README 下方。

## 开始游戏

- [网页版](https://ddz.fengxun.ca/)：电脑、手机均可直接打开。
- 人机对战：两位电脑牌友，玩家操作不限时。
- 好友房间：三人输入同一房间码，全部准备后开局。出牌 30 秒、叫地主 20 秒，超时或离桌由机器人代打。
- 提示只帮助选牌；支持完整牌型、牌局记录、春天计分、再来一局、断线恢复和可选音效。
- 房间和身份只在服务内存中保留，服务器重启会结束当前对局。匿名恢复令牌仅存在当前标签页的 sessionStorage；不同设备不共享身份。
- 没有账号、充值、金币、广告或现金输赢。

## 本地运行

需要 Go 1.26：`go run ./cmd/web`，打开 `http://127.0.0.1:9017`。不需要 Node、Redis 或终端客户端。

默认使用完整牌型规则机器人。设置 `DOUZERO_URL=http://localhost:2021` 可接入原项目的 ONNX 服务（见 douzero/README.md）。模型只接收自己的手牌和公开牌局信息。

环境变量：`WEB_ADDR` 默认 `127.0.0.1:9017`；`WEB_ORIGINS` 为逗号分隔的允许来源；`TRUST_PROXY=true` 仅用于被可信 Cloudflare Tunnel 隔离的来源。

生产配置为 `docker compose -f compose.web.yaml up -d --build`。该文件连接预先存在的 `n8n_default` 隧道网络，路由为 `ddz.fengxun.ca → http://fengxun-ddz:9017`，不开放主机端口。迁移其他服务器时替换该网络及域名。Go 服务与模型服务均以非 root 用户运行；模型只在内部网络提供 HTTP。

## 相对原项目的改动

- 新增 `cmd/web`、`internal/webgame`、`web`：服务端权威的 WebSocket 牌桌、匿名会话、私有手牌、随机洗牌、版本校验、重连、托管、房间回收和原生浏览器界面。
- 规则新增一致的飞机翅膀识别；提示和后备策略覆盖全部牌型，包含飞机、连对、顺子和四带二。
- DouZero 客户端允许注入完整规则后备策略；ONNX 推理线程限制为两个，适合共享服务器；修正推理状态中各位置最近出牌、过牌与炸弹数，使输入与原训练环境一致。
- 新增两个 Dockerfile 与独立 Compose 配置；原终端客户端部署流程不变。

验证：`go test ./internal/webgame ./internal/bot ./internal/game/...`，Linux 可加 `-race`。可选 `TEST_DOUZERO_URL=http://douzero:2021 go test ./internal/webgame -run TestLiveDouZeroGames -v` 验证十局真实模型对战。

## 美术与界面

茶室背景和三位虚构成年牌友头像由 OpenAI 图像工具原创生成，作为独立 WebP 静态素材随源码提供。没有复制其他游戏的美术、商标或音频。纸牌、花色与操作图标使用原生 HTML/CSS/SVG，点击音效为浏览器实时合成。

设计概念先确定桌面牌桌与大厅，再拆分空背景和头像。桌面保留暖色茶室、翡翠绿桌面、左右人物、上方底牌、中央出牌、底部手牌的构图；手机将手牌改为至少 44px 的完整可点网格，操作栏放在手牌下方。桌面操作栏上移，避免选牌抬起时挡住按钮。

原项目及衍生代码为 GPL-3.0。DouZero 的 Python 规则/特征代码源自 [kwai/DouZero](https://github.com/kwai/DouZero)，Apache-2.0，许可副本见 `douzero/LICENSE-DouZero`；预训练 ONNX 权重来自原作者的 [Hugging Face 镜像](https://huggingface.co/palemoky/douzero-baselines)，模型卡标注 Apache-2.0。原研究作者为 Kuaishou AI Platform 与 Texas A&M University。模型在构建时下载，不提交到本仓库。
