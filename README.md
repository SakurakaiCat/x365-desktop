<div align="center">
  <img src="assets/open-x365.svg" alt="Open X365" width="460">
</div>

# Open365VPN for Windows

Open365VPN 是一个基于 X365 协议的 Windows 桌面 VPN 客户端。X365 协议在
设计上受到其他 VPN / 代理隧道协议的启发，但协议格式与本项目全部实现均为
独立设计、独立开发，不基于任何第三方客户端的源码；与任何第三方 VPN
服务商（包括名称相近的服务商）没有任何隶属、授权或合作关系。
本项目不捆绑任何服务器节点或账号凭证。

## 功能

- X365 协议（REALITY TLS + gRPC 风格 chunked 传输）
- Wintun + tun2socks 全局接管，自动配置系统路由
- 系统代理（SOCKS5）模式可选
- 节点管理：批量导入 `x365://` URI、延迟测速、出口检测
- 系统托盘常驻

## 使用

应用不附带任何节点。启动后在节点页粘贴你自己的 `x365://` URI 导入。
TUN 模式需要管理员权限运行。

## 构建

```sh
npm --prefix frontend install
wails build
```

协议核心依赖 [core/](../core/README.md)。

## License

MIT（详见 [LICENSE](../LICENSE)）

## 免责声明

本项目仅供学习与研究网络协议使用。使用者应遵守所在地区的法律法规，
本项目作者不对任何使用行为承担责任。
