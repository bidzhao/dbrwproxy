# Database Proxy dbrwproxy 
[中文](README_CN.md)
## Introduction

**dbrwproxy** is a database proxy that supports read/write splitting for MySQL and PostgreSQL. It forwards modification requests (INSERT, UPDATE, DELETE, etc) to the master database instance and read-only SELECT queries to slave replicas based on configured weights. With this proxy, clients can achieve read/write splitting by simply connecting to dbrwproxy, without any code changes.

## Requirements

Databases must be configured for replication. Refer to [MySQL](https://www.postgresql.org/docs/current/warm-standby.html#STREAMING-REPLICATION)，[PostgreSQL](https://www.postgresql.org/docs/current/warm-standby.html#STREAMING-REPLICATION) for how to configure replication.

## Features

* Support Linux, Windows, MacOS
* Supports MySQL and PostgreSQL
* Proxies client login requests to master
* Configurable read weights for replicas
* Connection pooling for better replica efficiency
* Forwards transactions SELECTs to master for strong consistency

## Usage

### Compile
    go build

### Configure
    Modify the config.yml file to configure databases. You can configure both databases, or only one of them.

### Run
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

## License

This project is licensed under the [Mozilla Public License Version 2.0](https://raw.github.com/go-sql-driver/mysql/master/LICENSE)

Please read and understand the license terms before using this software.
