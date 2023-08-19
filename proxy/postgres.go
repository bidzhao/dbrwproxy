package proxy

import (
	"database/sql"
	"dbrwproxy/config"
	"fmt"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"io"
	"log"
	"math/rand"
	"net"
	"regexp"
	"strconv"
	"time"
)

var PostgresDBs []WeightedDB

type PostgresProxy struct {
	localAddr, remoteAddr *net.TCPAddr
	localConn, remoteConn *net.TCPConn
	dbs                   []WeightedDB
	totalWeight           int
	regex                 []*regexp.Regexp
	exit                  bool
	inTrans               bool
}

func StartPostgres(conf config.Proxy) {
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

	dbs, totalWeight := initDB(conf)
	if len(dbs) < 1 {
		log.Println("No active Secondary DB found for Proxy", conf.Name)
		return
	}
	PostgresDBs = dbs
	log.Println("PostgreSQL Proxy listening on", conf.Server.ProxyAddr)

	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			log.Println("Failed to accept connection", err)
			continue
		}

		p := &PostgresProxy{
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

func (p *PostgresProxy) service() {
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

func (p *PostgresProxy) handleInbound() {
	buff := make([]byte, 65535)
	for {
		n, err := p.localConn.Read(buff)
		if err != nil {
			if err != io.EOF {
				log.Println("Read local failed:", err)
			}
			return
		}
		flag, err := p.delegateSelect(buff[:n])
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

func (p *PostgresProxy) handleOutbound() {
	buff := make([]byte, 65535)
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

func (p *PostgresProxy) delegateSelect(buffer []byte) (bool, error) {
	if buffer[0] != 'Q' {
		return false, nil
	}
	end := len(buffer)
	if buffer[end-1] == 0 {
		end = end - 1
	}
	sql := string(buffer)[5:end]
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
	p.writeDataRow(db, sql)
	return true, nil
}

func (p *PostgresProxy) chooseByWeight() *sqlx.DB {
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

func (p *PostgresProxy) writeDataRow(db *sqlx.DB, query string) error {
	log.Println("Execute SQL -> ["+query+"]", "db", db)
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	desc, err := prepareRowDescription(rows)
	if err != nil {
		return err
	}
	buf := (&pgproto3.RowDescription{Fields: desc}).Encode(nil)
	_, err = p.localConn.Write(buf)
	if err != nil {
		return err
	}

	columns, err := rows.Columns()
	cols := len(columns)
	var count = 0
	for rows.Next() {
		row := make([][]byte, cols)
		rowPtr := make([]any, cols)
		for i := range row {
			rowPtr[i] = &row[i]
		}
		err := rows.Scan(rowPtr...)
		if err != nil {
			return err
		}
		buf = (&pgproto3.DataRow{Values: row}).Encode(nil)
		_, err = p.localConn.Write(buf)
		if err != nil {
			return err
		}
		count++
	}
	buf = (&pgproto3.CommandComplete{CommandTag: []byte("SELECT " + strconv.Itoa(count))}).Encode(nil)
	_, err = p.localConn.Write(buf)
	if err != nil {
		return err
	}
	buf = (&pgproto3.ReadyForQuery{TxStatus: 'I'}).Encode(nil)
	_, err = p.localConn.Write(buf)
	if err != nil {
		return err
	}

	if err = rows.Err(); err != nil {
		return err
	}
	return nil
}

func prepareRowDescription(rows *sql.Rows) ([]pgproto3.FieldDescription, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	columeTypeNames := make(map[string]string, len(cols))
	for i, col := range cols {
		columeTypeNames[col] = columnTypes[i].DatabaseTypeName()
	}
	if err != nil {
		return nil, err
	}
	var fd []pgproto3.FieldDescription
	for _, fieldName := range cols {
		fieldType := columeTypeNames[fieldName]
		var oid, size int
		if t, ok := dataTypeLookup[fieldType]; ok {
			oid = t.DataTypeOID
			size = t.DataTypeSize
		} else {
			oid = 17
			size = -1
		}
		fd = append(fd, pgproto3.FieldDescription{
			Name:                 []byte(fieldName),
			TableOID:             0,
			TableAttributeNumber: 0,
			DataTypeOID:          uint32(oid),
			DataTypeSize:         int16(size),
			TypeModifier:         -1,
			Format:               0,
		})
	}
	return fd, nil
}

type WeightedDB struct {
	Name   string
	Db     *sqlx.DB
	Weight int
}

func initDB(conf config.Proxy) ([]WeightedDB, int) {
	var dbs []WeightedDB
	total := 0
	for _, secondary := range conf.Db.Secondaries {
		if secondary.Weight <= 0 {
			continue
		}

		db, err := sqlx.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s application_name=pgproxy sslmode=disable",
			secondary.Host, secondary.Port, secondary.User, secondary.Password, secondary.DbName))
		if err != nil {
			log.Println("Can NOT open Secondary DB", secondary.Name, "of Proxy", conf.Name)
			continue
		}
		if secondary.MaxIdleConnCount > 0 {
			db.SetMaxIdleConns(secondary.MaxIdleConnCount)
		} else {
			db.SetMaxIdleConns(1)
		}
		if secondary.MaxOpenConnsCount > 0 {
			db.SetMaxOpenConns(secondary.MaxOpenConnsCount)
		} else {
			db.SetMaxOpenConns(10)
		}
		if secondary.ConnMaxLifetime > 0 {
			db.SetConnMaxLifetime(time.Duration(secondary.ConnMaxLifetime) * time.Second)
		} else {
			db.SetConnMaxLifetime(60 * time.Second)
		}
		dbs = append(dbs, WeightedDB{Name: secondary.Name, Db: db, Weight: secondary.Weight})
		total += secondary.Weight
	}
	return dbs, total
}

type pgDataType struct {
	DataTypeOID  int
	DataTypeSize int
}

var dataTypeLookup = makeDataTypeLookup()

func makeDataTypeLookup() map[string]pgDataType {
	return map[string]pgDataType{
		"BOOL":     {DataTypeOID: 16, DataTypeSize: 1},
		"BYTEA":    {DataTypeOID: 17, DataTypeSize: -1},
		"CHAR":     {DataTypeOID: 18, DataTypeSize: 1},
		"INT8":     {DataTypeOID: 20, DataTypeSize: 8},
		"INT2":     {DataTypeOID: 21, DataTypeSize: 2},
		"INT4":     {DataTypeOID: 23, DataTypeSize: 4},
		"REGPROC":  {DataTypeOID: 24, DataTypeSize: 4},
		"_ACLITEM": {DataTypeOID: 25, DataTypeSize: -1},
		"NAME":     {DataTypeOID: 25, DataTypeSize: -1},
		"TEXT":     {DataTypeOID: 25, DataTypeSize: -1},
		"VARCHAR":  {DataTypeOID: 25, DataTypeSize: -1},
		"OID":      {DataTypeOID: 26, DataTypeSize: 4},
		"TID":      {DataTypeOID: 27, DataTypeSize: 6},
		"XID":      {DataTypeOID: 28, DataTypeSize: 4},
		"CID":      {DataTypeOID: 29, DataTypeSize: 4},
		"JSON":     {DataTypeOID: 114, DataTypeSize: -1},
		"XML":      {DataTypeOID: 142, DataTypeSize: -1},
		"POINT":    {DataTypeOID: 600, DataTypeSize: 16},
		"FLOAT4":   {DataTypeOID: 700, DataTypeSize: 4},
		"FLOAT8":   {DataTypeOID: 701, DataTypeSize: 8},
	}
}
