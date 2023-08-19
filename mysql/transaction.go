package mysql

type mysqlTx struct {
	mc *MysqlConn
}

func (tx *mysqlTx) Commit() (err error) {
	if tx.mc == nil || tx.mc.closed.Load() {
		return ErrInvalidConn
	}
	err = tx.mc.exec("COMMIT")
	tx.mc = nil
	return
}

func (tx *mysqlTx) Rollback() (err error) {
	if tx.mc == nil || tx.mc.closed.Load() {
		return ErrInvalidConn
	}
	err = tx.mc.exec("ROLLBACK")
	tx.mc = nil
	return
}
