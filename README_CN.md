# 数据库代理 dbrwproxy
[English](README_cn.md)
## 简介

**dwrwproxy**是一个支持 MySQL 和 PostgreSQL 读写分离的代理软件，用于转发数据库查询和更新请求，将修改请求（INSERT，UPDATE，DELETE等）发送到数据库主实例，并按照权重将SELECT查询转到数据库只读从库。客户端不需要修改代码逻辑，只要连接到dbrwproxy即可简单地实现读写分离。

## 前提条件
数据库已配置主从复制。具体配置方法可参考[MySQL](https://www.postgresql.org/docs/current/warm-standby.html#STREAMING-REPLICATION)，[PostgreSQL] (https://www.postgresql.org/docs/current/warm-standby.html#STREAMING-REPLICATION)

## 功能
* 跨平台，支持Linux，Windows，MacOS
* 支持MySQL 和 PostgreSQL
* 当proxy后端为MySQL时，用户使用MySQL协议访问proxy。当proxy后端为PostgreSQL时，用户使用PostgreSQL协议访问proxy。
* 代理客户端登录请求到主库
* 支持设置从库的权重
* 代理使用连接池管理从库连接，效率更高
* 事务中的SELECT查询代理到主库，以保证数据的强一致性

## 使用方法

### 编译代码：
    使用 go build 命令进行编译。

### 配置文件：
    修改 config.yml 配置文件，其中可以配置 Postgres 和 MySQL 两种类型的数据库。你可以同时配置两种数据库，也可以只配置其中一种。

### 运行：
    ./dbrwproxy -c config.yml

### 配置文件示例

```
PostgreSQL:
  - Proxy:
    Name: p1
    ServerConfig:
      ProxyAddr: "127.0.0.1:15432"
    DB:
      Main:
        Addr: "127.0.0.1:5432"
      Secondaries:
        - Secondary:
          Name: "A"
          Host: "127.0.0.1"
          Port: 5432
          User: "postgres"
          Password: "12345678"
          DbName: "mydb"
          Weight: 100
          MaxIdleConnCount: 1
          MaxOpenConnsCount: 10
          ConnMaxLifetime: 60
        - Secondary:
          Name: "B"
          Host: "127.0.0.1"
          Port: 5442
          User: "postgres"
          Password: "12345678"
          DbName: "mydb"
          Weight: 300
          MaxIdleConnCount: 1
          MaxOpenConnsCount: 10
          ConnMaxLifetime: 60

MySQL:
  - Proxy:
    Name: p2
    ServerConfig:
      ProxyAddr: "0.0.0.0:13306"
    DB:
      Main:
        Addr: "127.0.0.1:3306"
      Secondaries:
        - Secondary:
          Name: "E"
          Host: "127.0.0.1"
          Port: 3306
          User: "root"
          Password: "12345678"
          DbName: "mydb"
          Weight: 100
          MaxIdleConnCount: 1
          MaxOpenConnsCount: 10
          ConnMaxLifetime: 60
        - Secondary:
          Name: "F"
          Host: "127.0.0.1"
          Port: 3316
          User: "root"
          Password: "12345678"
          DbName: "mydb"
          Weight: 300
          MaxIdleConnCount: 1
          MaxOpenConnsCount: 10
          ConnMaxLifetime: 60
```

### 许可证

本软件使用 Mozilla Public License（MPL）进行许可。

注意：请在使用本软件之前确保您已经阅读并理解了许可证中的条款和条件。
