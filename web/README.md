# 前端目录

Vue 3 + TypeScript + Naive UI。生产资源由 Vite 构建到 `dist`，再嵌入 Go 可执行文件。

```text
src/
├── App.vue                 # 全局主题、会话恢复和登录入口
├── api/client.ts           # 请求、错误响应和文件下载
├── layouts/AdminLayout.vue # 导航、通用提示、页面切换与定时刷新
├── views/
│   ├── LoginView.vue
│   ├── DashboardView.vue
│   ├── users/              # 用户列表、用户弹窗及表单类型
│   ├── routes/             # 路由列表、路由弹窗及表单类型
│   ├── OnlineView.vue
│   ├── SystemView.vue
│   ├── AuditView.vue
│   └── SettingsView.vue
├── components/AuditTable.vue # 仪表盘和审计页共用的表格
├── composables/useAppContext.ts # 会话、已保存设置、操作状态及错误处理
├── types/index.ts          # 后端响应和页面接口类型
├── utils/                  # 时间/流量格式、表格按钮及状态名称
└── styles/main.css         # 共用样式与响应式布局
```

页面拥有自己的查询、筛选、分页、表格和业务操作。用户与路由弹窗负责表单输入，通过事件交给所属页面提交；页面等待保存及重新查询完成后结束操作状态。设置页使用本地表单，保存成功后再更新共用设置。

各管理页通过 `defineExpose` 暴露 `refresh()`。布局在首次登录、切换页面和手动刷新时调用；仪表盘、用户、在线和系统页按配置周期自动刷新。带弹窗的页面同时暴露 `isEditing`，避免编辑期间触发自动刷新。

共用上下文只提供会话和跨页面状态，不存放用户列表、路由、表单或弹窗。页面导航沿用原有单页切换方式，没有新增 Router 或状态管理依赖。新增功能应落在对应页面目录；多个页面确实共用时再抽取组件。

```sh
npm run dev
npm run build  # 同时运行 vue-tsc 类型检查
```
