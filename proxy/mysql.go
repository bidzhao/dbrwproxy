package proxy

import (
	"database/sql/driver"
	"dbrwproxy/config"
	"dbrwproxy/mysql"
	"dbrwproxy/pool"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"time"
)

var MysqlDBs []WeightedMysqlDB

type MysqlProxy struct {
	localAddr, remoteAddr *net.TCPAddr
	localConn, remoteConn *net.TCPConn
	dbs                   []WeightedMysqlDB
	totalWeight           int
	regex                 []*regexp.Regexp
	exit                  bool
	inTrans               bool
}

type WeightedMysqlDB struct {
	Name   string
	Db     *pool.ConnectionPool
	Weight int
}

func StartMysql(conf config.Proxy) {
	localAddr, err := net.ResolveTCPAddr("tcp", conf.Server.ProxyAddr)
	if err != nil {
		log.Fatalln("Failed to resolve host ", err)
		return
	}
	remoteAddr, err := net.ResolveTCPAddr("tcp", conf.Db.Main.Addr)
	if err != nil {
		log.Fatalln("Failed to resolve remote host ", err)
		return
	}
	listener, err := net.ListenTCP("tcp", localAddr)
	if err != nil {
		log.Fatalln("Failed to listen on ", localAddr, "Error:", err)
		return
	}

	dbs, totalWeight := initMysqlDB(conf)
	if len(dbs) < 1 {
		log.Println("No active Secondary DB found for Proxy", conf.Name)
		return
	}
	MysqlDBs = dbs
	log.Println("Mysql Proxy listening on", conf.Server.ProxyAddr)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Println("Failed to accept connection", err)
			continue
		}

		p := &MysqlProxy{
			localConn:   conn,
			localAddr:   localAddr,
			remoteAddr:  remoteAddr,
			dbs:         dbs,
			totalWeight: totalWeight,
		}
		p.regex = initRegexp()
		go p.service()
	}
}

func (p *MysqlProxy) service() {
	defer p.localConn.Close()
	conn, err := net.DialTCP("tcp", nil, p.remoteAddr)
	if err != nil {
		log.Printf("Remote connection failed:", err)
		return
	}
	p.remoteConn = conn
	defer p.remoteConn.Close()
	go p.handleOutbound()
	p.handleInbound()
	p.exit = true
}

func (p *MysqlProxy) handleInbound() {
	buff := make([]byte, 65536)
	for {
		n, err := p.localConn.Read(buff)
		if err != nil {
			if err != io.EOF {
				log.Println("Read local failed:", err)
			}
			return
		}
		flag, err := p.delegateSelect(n, buff[:n])
		if !flag {
			if err != nil {
				log.Println(err)
				err = p.remoteConn.Close()
				if err != nil {
					log.Fatalln(err)
				}
				return
			}
			n, err = p.remoteConn.Write(buff[0:n])
			if err != nil {
				log.Println("Write failed:", err)
				return
			}
		}
	}
}

func (p *MysqlProxy) delegateSelect(n int, buffer []byte) (bool, error) {
	if n <= 5 || buffer[4] != 3 {
		return false, nil
	}
	sql := strings.Trim(string(buffer[5:n]), " \r\n")
	matchSelect := p.regex[0].MatchString(sql)
	if p.inTrans || !matchSelect {
		matchTranBegin := p.regex[1].MatchString(sql)
		if matchTranBegin {
			p.inTrans = true
		}
		if p.inTrans {
			matchTranEnd := p.regex[2].MatchString(sql)
			if matchTranEnd {
				p.inTrans = false
			}
		}
		log.Println("Choose main")
		log.Println("Execute SQL -> [" + sql + "]")
		return false, nil
	}
	db := p.chooseByWeight()
	conn, err := db.Get()
	if err != nil {
		return true, err
	}
	defer db.Put(conn)
	p.writeDataRow(conn, sql)
	return true, nil
}

func (p *MysqlProxy) chooseByWeight() *pool.ConnectionPool {
	randomNum := rand.Intn(p.totalWeight)
	currentWeight := 0
	for _, element := range p.dbs {
		currentWeight += element.Weight
		if randomNum < currentWeight {
			log.Println("Choose", element.Name)
			return element.Db
		}
	}
	return p.dbs[0].Db
}

func (p *MysqlProxy) writeDataRow(db *mysql.MysqlConn, query string) error {
	log.Println("Execute SQL -> [" + query + "]")
	rows, err := db.Query1(query, p.localConn)
	if err != nil {
		return err
	}

	values := make([]driver.Value, len(rows.Columns()))
	for {
		if rows.Next1(values, p.localConn) == io.EOF {
			break
		}
	}

	return nil
}

func (p *MysqlProxy) handleOutbound() {
	buff := make([]byte, 65536)
	for {
		n, err := p.remoteConn.Read(buff)
		if err != nil {
			if err != io.EOF && !p.exit {
				log.Println("Read remote failed:", err)
			}
			return
		}
		if n < 1 {
			continue
		}
		n, err = p.localConn.Write(buff[0:n])
		if err != nil {
			log.Println("Write failed:", err)
			return
		}
	}
	log.Println("handle outbound returned!!!")
}

func initMysqlDB(conf config.Proxy) ([]WeightedMysqlDB, int) {
	var dbs []WeightedMysqlDB
	total := 0
	for _, secondary := range conf.Db.Secondaries {
		if secondary.Weight <= 0 {
			continue
		}
		dsn := fmt.Sprintf("%s:%s@(%s:%d)/%s",
			secondary.User, secondary.Password, secondary.Host, secondary.Port, secondary.DbName)
		connector, err := mysql.CreateConnector(dsn)
		if err != nil {
			log.Println("Can NOT open Secondary DB", secondary.Name, "of Proxy", conf.Name, err)
			continue
		}
		min := 1
		max := 10
		lifeTime := 60 * time.Second
		if secondary.MaxIdleConnCount > 0 {
			min = secondary.MaxIdleConnCount
		}
		if secondary.MaxOpenConnsCount > 0 {
			max = secondary.MaxOpenConnsCount
		}
		if secondary.ConnMaxLifetime > 0 {
			lifeTime = time.Duration(secondary.ConnMaxLifetime) * time.Second
		}

		connPool := pool.NewConnectionPool(connector, min, max, lifeTime)
		dbs = append(dbs, WeightedMysqlDB{Name: secondary.Name, Db: connPool, Weight: secondary.Weight})
		total += secondary.Weight
	}
	return dbs, total
}

func initRegexp() []*regexp.Regexp {
	pattern := []string{`(?i)^(select)`, `(?i)^(begin|start transaction)`, `(?i)^(commit|rollback)`}
	return []*regexp.Regexp{regexp.MustCompile(pattern[0]), regexp.MustCompile(pattern[1]), regexp.MustCompile(pattern[2])}
}
