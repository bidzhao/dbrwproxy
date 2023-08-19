package mysql

import (
	"database/sql/driver"
	"io"
	"math"
	"net"
	"reflect"
)

type resultSet struct {
	columns     []mysqlField
	columnNames []string
	done        bool
}

type MysqlRows struct {
	mc     *MysqlConn
	rs     resultSet
	finish func()
}

type BinaryRows struct {
	MysqlRows
}

type TextRows struct {
	MysqlRows
}

func (rows *MysqlRows) Columns() []string {
	if rows.rs.columnNames != nil {
		return rows.rs.columnNames
	}

	columns := make([]string, len(rows.rs.columns))
	if rows.mc != nil && rows.mc.cfg.ColumnsWithAlias {
		for i := range columns {
			if tableName := rows.rs.columns[i].tableName; len(tableName) > 0 {
				columns[i] = tableName + "." + rows.rs.columns[i].name
			} else {
				columns[i] = rows.rs.columns[i].name
			}
		}
	} else {
		for i := range columns {
			columns[i] = rows.rs.columns[i].name
		}
	}

	rows.rs.columnNames = columns
	return columns
}

func (rows *MysqlRows) ColumnTypeDatabaseTypeName(i int) string {
	return rows.rs.columns[i].typeDatabaseName()
}

// func (rows *MysqlRows) ColumnTypeLength(i int) (length int64, ok bool) {
// 	return int64(rows.rs.columns[i].length), true
// }

func (rows *MysqlRows) ColumnTypeNullable(i int) (nullable, ok bool) {
	return rows.rs.columns[i].flags&flagNotNULL == 0, true
}

func (rows *MysqlRows) ColumnTypePrecisionScale(i int) (int64, int64, bool) {
	column := rows.rs.columns[i]
	decimals := int64(column.decimals)

	switch column.fieldType {
	case fieldTypeDecimal, fieldTypeNewDecimal:
		if decimals > 0 {
			return int64(column.length) - 2, decimals, true
		}
		return int64(column.length) - 1, decimals, true
	case fieldTypeTimestamp, fieldTypeDateTime, fieldTypeTime:
		return decimals, decimals, true
	case fieldTypeFloat, fieldTypeDouble:
		if decimals == 0x1f {
			return math.MaxInt64, math.MaxInt64, true
		}
		return math.MaxInt64, decimals, true
	}

	return 0, 0, false
}

func (rows *MysqlRows) ColumnTypeScanType(i int) reflect.Type {
	return rows.rs.columns[i].scanType()
}

func (rows *MysqlRows) Close() (err error) {
	if f := rows.finish; f != nil {
		f()
		rows.finish = nil
	}

	mc := rows.mc
	if mc == nil {
		return nil
	}
	if err := mc.error(); err != nil {
		return err
	}

	// flip the buffer for this connection if we need to drain it.
	// note that for a successful query (i.e. one where rows.next()
	// has been called until it returns false), `rows.mc` will be nil
	// by the time the user calls `(*Rows).Close`, so we won't reach this
	// see: https://github.com/golang/go/commit/651ddbdb5056ded455f47f9c494c67b389622a47
	mc.buf.flip()

	// Remove unread packets from stream
	if !rows.rs.done {
		err = mc.readUntilEOF()
	}
	if err == nil {
		handleOk := mc.clearResult()
		if err = handleOk.discardResults(); err != nil {
			return err
		}
	}

	rows.mc = nil
	return err
}

func (rows *MysqlRows) HasNextResultSet() (b bool) {
	if rows.mc == nil {
		return false
	}
	return rows.mc.status&statusMoreResultsExists != 0
}

func (rows *MysqlRows) nextResultSet() (int, error) {
	if rows.mc == nil {
		return 0, io.EOF
	}
	if err := rows.mc.error(); err != nil {
		return 0, err
	}

	// Remove unread packets from stream
	if !rows.rs.done {
		if err := rows.mc.readUntilEOF(); err != nil {
			return 0, err
		}
		rows.rs.done = true
	}

	if !rows.HasNextResultSet() {
		rows.mc = nil
		return 0, io.EOF
	}
	rows.rs = resultSet{}
	// rows.mc.affectedRows and rows.mc.insertIds accumulate on each call to
	// nextResultSet.
	return rows.mc.resultUnchanged().readResultSetHeaderPacket()
}

func (rows *MysqlRows) nextResultSet1(dstConn *net.TCPConn) (int, error) {
	if rows.mc == nil {
		return 0, io.EOF
	}
	if err := rows.mc.error(); err != nil {
		return 0, err
	}

	// Remove unread packets from stream
	if !rows.rs.done {
		if err := rows.mc.readUntilEOF1(dstConn); err != nil {
			return 0, err
		}
		rows.rs.done = true
	}

	if !rows.HasNextResultSet() {
		rows.mc = nil
		return 0, io.EOF
	}
	rows.rs = resultSet{}
	// rows.mc.affectedRows and rows.mc.insertIds accumulate on each call to
	// nextResultSet.
	return rows.mc.resultUnchanged().readResultSetHeaderPacket1(dstConn)
}

func (rows *MysqlRows) nextNotEmptyResultSet() (int, error) {
	for {
		resLen, err := rows.nextResultSet()
		if err != nil {
			return 0, err
		}

		if resLen > 0 {
			return resLen, nil
		}

		rows.rs.done = true
	}
}

func (rows *MysqlRows) nextNotEmptyResultSet1(dstConn *net.TCPConn) (int, error) {
	for {
		resLen, err := rows.nextResultSet1(dstConn)
		if err != nil {
			return 0, err
		}

		if resLen > 0 {
			return resLen, nil
		}

		rows.rs.done = true
	}
}

func (rows *BinaryRows) NextResultSet() error {
	resLen, err := rows.nextNotEmptyResultSet()
	if err != nil {
		return err
	}

	rows.rs.columns, err = rows.mc.readColumns(resLen)
	return err
}

func (rows *BinaryRows) NextResultSet1(dstConn *net.TCPConn) error {
	resLen, err := rows.nextNotEmptyResultSet1(dstConn)
	if err != nil {
		return err
	}

	rows.rs.columns, err = rows.mc.readColumns1(resLen, dstConn)
	return err
}

func (rows *BinaryRows) Next(dest []driver.Value) error {
	if mc := rows.mc; mc != nil {
		if err := mc.error(); err != nil {
			return err
		}

		// Fetch next row from stream
		return rows.readRow(dest)
	}
	return io.EOF
}

func (rows *TextRows) NextResultSet() (err error) {
	resLen, err := rows.nextNotEmptyResultSet()
	if err != nil {
		return err
	}

	rows.rs.columns, err = rows.mc.readColumns(resLen)
	return err
}

func (rows *TextRows) NextResultSet1(dstConn *net.TCPConn) (err error) {
	resLen, err := rows.nextNotEmptyResultSet1(dstConn)
	if err != nil {
		return err
	}

	rows.rs.columns, err = rows.mc.readColumns1(resLen, dstConn)
	return err
}

func (rows *TextRows) Next(dest []driver.Value) error {
	if mc := rows.mc; mc != nil {
		if err := mc.error(); err != nil {
			return err
		}

		// Fetch next row from stream
		return rows.readRow(dest)
	}
	return io.EOF
}

func (rows *TextRows) Next1(dest []driver.Value, dstConn *net.TCPConn) error {
	if mc := rows.mc; mc != nil {
		if err := mc.error(); err != nil {
			return err
		}

		// Fetch next row from stream
		return rows.readRow1(dest, dstConn)
	}
	return io.EOF
}
