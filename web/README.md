# One API 的前端界面

> 每个文件夹代表一个主题，欢迎提交你的主题

## 提交新的主题

> 欢迎在页面底部保留你和 One API 的版权信息以及指向链接

1. 在 `web` 文件夹下新建一个文件夹，文件夹名为主题名。
2. 把你的主题文件放到这个文件夹下。
3. 修改你的 `package.json` 文件，把 `build` 命令改为：`"build": "react-scripts build && mv -f build ../build/default"`，其中 `default` 为你的主题名。
4. 修改 `common/constants.go` 中的 `ValidThemes`，把你的主题名称注册进去。
5. 修改 `web/THEMES` 文件，这里也需要同步修改。
