# 哆啦魔改魔改版


## 特性
- 对于某些按钮做了移动端适配
- 增加了tcp预链接的支持

#操作说明
- 务必选择安装，不要更新
- 完事以后网页缓存要清理
- 节点机器要重新点击安装，然后使用贴出的代码进行节点安装，我无法控制老版本已经安装好的后端进行重新拉我的更新（暂时）。

#### 面板端部署
```bash
curl -L https://raw.githubusercontent.com/Xeloan/flvx-tcppreconn/main/panel_install.sh -o panel_install.sh && chmod +x panel_install.sh && ./panel_install.sh
```


#### 节点端部署
```bash
curl -L https://raw.githubusercontent.com/Xeloan/flvx-tcppreconn/main/install.sh -o install.sh && chmod +x install.sh && ./install.sh
```


## 免责声明

本项目为纯瞎搞，但是经过了非常多的测试。基本稳定。
