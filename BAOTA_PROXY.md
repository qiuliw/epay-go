# 宝塔反代

站点：`pay.admincloudai.com`  
反代目标：`http://127.0.0.1:18080`

## 端口

| 端口 | 服务 |
|------|------|
| 18080 | 前端（整站反代这个） |
| 8080 | 后端 API |
| 5432 | PostgreSQL |
| 6379 | Redis |

## 后台

- 地址：`https://pay.admincloudai.com/admin/login`
- 账号：`admin` / `AdminCloud#Pay2026`

虎皮椒回调：`https://pay.admincloudai.com/api/pay/notify/xh-alipay`
