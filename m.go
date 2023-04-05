/*
 * Copyright (c) 2000-2018, 达梦数据库有限公司.
 * All rights reserved.
 */
package dm

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"github.com/truxcoder/dm/parser"
	"fmt"
	"golang.org/x/text/encoding"
	"sync/atomic"
)

type DmConnection struct {
	filterable

	dmConnector        *DmConnector
	Access             *dm_build_332
	stmtMap            map[int32]*DmStatement
	stmtPool           []stmtPoolInfo
	lastExecInfo       *execRetInfo
	lexer              *parser.Lexer
	encode             encoding.Encoding
	encodeBuffer       *bytes.Buffer
	transformReaderDst []byte
	transformReaderSrc []byte

	serverEncoding     string
	GlobalServerSeries int
	ServerVersion      string
	Malini2            bool
	Execute2           bool
	LobEmptyCompOrcl   bool
	IsoLevel           int32
	ReadOnly           bool
	NewLobFlag         bool
	sslEncrypt         int
	MaxRowSize         int32
	DDLAutoCommit      bool
	BackslashEscape    bool
	SvrStat            int32
	SvrMode            int32
	ConstParaOpt       bool
	DbTimezone         int16
	LifeTimeRemainder  int16
	InstanceName       string
	Schema             string
	LastLoginIP        string
	LastLoginTime      string
	FailedAttempts     int32
	LoginWarningID     int32
	GraceTimeRemainder int32
	Guid               string
	DbName             string
	StandbyHost        string
	StandbyPort        int32
	StandbyCount       int32
	SessionID          int64
	OracleDateLanguage byte
	FormatDate         string
	FormatTimestamp    string
	FormatTimestampTZ  string
	FormatTime         string
	FormatTimeTZ       string
	Local              bool
	MsgVersion         int32
	TrxStatus          int32
	dscControl         bool
	trxFinish          bool
	sessionID          int64
	autoCommit         bool
	isBatch            bool

	watching bool
	watcher  chan<- context.Context
	closech  chan struct{}
	finished chan<- struct{}
	canceled atomicError
	closed   atomicBool
}

func (conn *DmConnection) setTrxFinish(status int32) {
	switch status & Dm_build_721 {
	case Dm_build_718, Dm_build_719, Dm_build_720:
		conn.trxFinish = true
	default:
		conn.trxFinish = false
	}
}

func (dmConn *DmConnection) init() {
	if dmConn.dmConnector.stmtPoolMaxSize > 0 {
		dmConn.stmtPool = make([]stmtPoolInfo, 0, dmConn.dmConnector.stmtPoolMaxSize)
	}

	dmConn.stmtMap = make(map[int32]*DmStatement)
	dmConn.DbTimezone = 0
	dmConn.GlobalServerSeries = 0
	dmConn.MaxRowSize = 0
	dmConn.LobEmptyCompOrcl = false
	dmConn.ReadOnly = false
	dmConn.DDLAutoCommit = false
	dmConn.ConstParaOpt = false
	dmConn.IsoLevel = -1
	dmConn.sessionID = -1
	dmConn.Malini2 = true
	dmConn.NewLobFlag = true
	dmConn.Execute2 = true
	dmConn.serverEncoding = ENCODING_GB18030
	dmConn.TrxStatus = Dm_build_669
	dmConn.OracleDateLanguage = byte(Locale)
	dmConn.lastExecInfo = NewExceInfo()
	dmConn.MsgVersion = Dm_build_603

	dmConn.idGenerator = dmConnIDGenerator
}

func (dmConn *DmConnection) reset() {
	dmConn.DbTimezone = 0
	dmConn.GlobalServerSeries = 0
	dmConn.MaxRowSize = 0
	dmConn.LobEmptyCompOrcl = false
	dmConn.ReadOnly = false
	dmConn.DDLAutoCommit = false
	dmConn.ConstParaOpt = false
	dmConn.IsoLevel = -1
	dmConn.sessionID = -1
	dmConn.Malini2 = true
	dmConn.NewLobFlag = true
	dmConn.Execute2 = true
	dmConn.serverEncoding = ENCODING_GB18030
	dmConn.TrxStatus = Dm_build_669
}

func (dc *DmConnection) checkClosed() error {
	if dc.closed.IsSet() {
		return driver.ErrBadConn
	}

	return nil
}

func (dc *DmConnection) executeInner(query string, execType int16) (interface{}, error) {

	stmt, err := NewDmStmt(dc, query)

	if err != nil {
		return nil, err
	}

	if execType == Dm_build_686 {
		defer stmt.close()
	}

	stmt.innerUsed = true
	if stmt.dmConn.dmConnector.escapeProcess {
		stmt.nativeSql, err = stmt.dmConn.escape(stmt.nativeSql, stmt.dmConn.dmConnector.keyWords)
		if err != nil {
			stmt.close()
			return nil, err
		}
	}

	var optParamList []OptParameter

	if stmt.dmConn.ConstParaOpt {
		optParamList = make([]OptParameter, 0)
		stmt.nativeSql, optParamList, err = stmt.dmConn.execOpt(stmt.nativeSql, optParamList, stmt.dmConn.getServerEncoding())
		if err != nil {
			stmt.close()
			optParamList = nil
		}
	}

	if execType == Dm_build_685 && dc.dmConnector.enRsCache {
		rpv, err := rp.get(stmt, query)
		if err != nil {
			return nil, err
		}

		if rpv != nil {
			stmt.execInfo = rpv.execInfo
			dc.lastExecInfo = rpv.execInfo
			return newDmRows(rpv.getResultSet(stmt)), nil
		}
	}

	var info *execRetInfo

	if optParamList != nil && len(optParamList) > 0 {
		info, err = dc.Access.Dm_build_411(stmt, optParamList)
		if err != nil {
			stmt.nativeSql = query
			info, err = dc.Access.Dm_build_417(stmt, execType)
		}
	} else {
		info, err = dc.Access.Dm_build_417(stmt, execType)
	}

	if err != nil {
		stmt.close()
		return nil, err
	}
	dc.lastExecInfo = info

	if info.hasResultSet {
		return newDmRows(newInnerRows(0, stmt, info)), nil
	} else {
		return newDmResult(stmt, info), nil
	}
}

func g2dbIsoLevel(isoLevel int32) int32 {
	switch isoLevel {
	case 1:
		return Dm_build_673
	case 2:
		return Dm_build_674
	case 4:
		return Dm_build_675
	case 6:
		return Dm_build_676
	default:
		return -1
	}
}

func (dc *DmConnection) Begin() (driver.Tx, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.begin()
	} else {
		return dc.filterChain.reset().DmConnectionBegin(dc)
	}
}

func (dc *DmConnection) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.beginTx(ctx, opts)
	}
	return dc.filterChain.reset().DmConnectionBeginTx(dc, ctx, opts)
}

func (dc *DmConnection) Commit() error {
	if len(dc.filterChain.filters) == 0 {
		return dc.commit()
	} else {
		return dc.filterChain.reset().DmConnectionCommit(dc)
	}
}

func (dc *DmConnection) Rollback() error {
	if len(dc.filterChain.filters) == 0 {
		return dc.rollback()
	} else {
		return dc.filterChain.reset().DmConnectionRollback(dc)
	}
}

func (dc *DmConnection) Close() error {
	if len(dc.filterChain.filters) == 0 {
		return dc.close()
	} else {
		return dc.filterChain.reset().DmConnectionClose(dc)
	}
}

func (dc *DmConnection) Ping(ctx context.Context) error {
	if len(dc.filterChain.filters) == 0 {
		return dc.ping(ctx)
	} else {
		return dc.filterChain.reset().DmConnectionPing(dc, ctx)
	}
}

func (dc *DmConnection) Exec(query string, args []driver.Value) (driver.Result, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.exec(query, args)
	}
	return dc.filterChain.reset().DmConnectionExec(dc, query, args)
}

func (dc *DmConnection) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.execContext(ctx, query, args)
	}
	return dc.filterChain.reset().DmConnectionExecContext(dc, ctx, query, args)
}

func (dc *DmConnection) Query(query string, args []driver.Value) (driver.Rows, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.query(query, args)
	}
	return dc.filterChain.reset().DmConnectionQuery(dc, query, args)
}

func (dc *DmConnection) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.queryContext(ctx, query, args)
	}
	return dc.filterChain.reset().DmConnectionQueryContext(dc, ctx, query, args)
}

func (dc *DmConnection) Prepare(query string) (driver.Stmt, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.prepare(query)
	}
	return dc.filterChain.reset().DmConnectionPrepare(dc, query)
}

func (dc *DmConnection) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if len(dc.filterChain.filters) == 0 {
		return dc.prepareContext(ctx, query)
	}
	return dc.filterChain.reset().DmConnectionPrepareContext(dc, ctx, query)
}

func (dc *DmConnection) ResetSession(ctx context.Context) error {
	if len(dc.filterChain.filters) == 0 {
		return dc.resetSession(ctx)
	}
	return dc.filterChain.reset().DmConnectionResetSession(dc, ctx)
}

func (dc *DmConnection) CheckNamedValue(nv *driver.NamedValue) error {
	if len(dc.filterChain.filters) == 0 {
		return dc.checkNamedValue(nv)
	}
	return dc.filterChain.reset().DmConnectionCheckNamedValue(dc, nv)
}

func (dc *DmConnection) begin() (*DmConnection, error) {
	return dc.beginTx(context.Background(), driver.TxOptions{driver.IsolationLevel(sql.LevelDefault), false})
}

func (dc *DmConnection) beginTx(ctx context.Context, opts driver.TxOptions) (*DmConnection, error) {
	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	if err := dc.watchCancel(ctx); err != nil {
		return nil, err
	}
	defer dc.finish()

	if sql.IsolationLevel(opts.Isolation) == sql.LevelDefault {
		opts.Isolation = driver.IsolationLevel(sql.LevelReadCommitted)
	}

	dc.ReadOnly = opts.ReadOnly

	if dc.IsoLevel == int32(opts.Isolation) {
		return dc, nil
	}

	switch sql.IsolationLevel(opts.Isolation) {
	case sql.LevelDefault:
		return dc, nil
	case sql.LevelReadUncommitted, sql.LevelReadCommitted, sql.LevelSerializable:
		dc.IsoLevel = int32(opts.Isolation)
	case sql.LevelRepeatableRead:
		if dc.CompatibleMysql() {
			dc.IsoLevel = int32(sql.LevelReadCommitted)
		} else {
			return nil, ECGO_INVALID_TRAN_ISOLATION.throw()
		}
	default:
		return nil, ECGO_INVALID_TRAN_ISOLATION.throw()
	}

	err = dc.Access.Dm_build_471(dc)
	if err != nil {
		return nil, err
	}
	return dc, nil
}

func (dc *DmConnection) commit() error {
	err := dc.checkClosed()
	if err != nil {
		return err
	}

	defer func() {
		dc.autoCommit = dc.dmConnector.autoCommit
	}()

	if !dc.autoCommit {
		err = dc.Access.Commit()
		if err != nil {
			return err
		}
		dc.trxFinish = true
		return nil
	} else if !dc.dmConnector.alwayseAllowCommit {
		return ECGO_COMMIT_IN_AUTOCOMMIT_MODE.throw()
	}

	return nil
}

func (dc *DmConnection) rollback() error {
	err := dc.checkClosed()
	if err != nil {
		return err
	}

	defer func() {
		dc.autoCommit = dc.dmConnector.autoCommit
	}()

	if !dc.autoCommit {
		err = dc.Access.Rollback()
		if err != nil {
			return err
		}
		dc.trxFinish = true
		return nil
	} else if !dc.dmConnector.alwayseAllowCommit {
		return ECGO_ROLLBACK_IN_AUTOCOMMIT_MODE.throw()
	}

	return nil
}

func (dc *DmConnection) reconnect() error {
	err := dc.Access.Close()
	if err != nil {
		return err
	}

	for _, stmt := range dc.stmtMap {
		stmt.closed = true
		for id, _ := range stmt.rsMap {
			delete(stmt.rsMap, id)
		}
	}

	if dc.stmtPool != nil {
		dc.stmtPool = dc.stmtPool[:0]
	}

	dc.dmConnector.reConnection = dc

	if dc.dmConnector.group != nil {
		_, err = dc.dmConnector.group.connect(dc.dmConnector)
		if err != nil {
			return err
		}
	} else {
		_, err = dc.dmConnector.connect(context.Background())
	}

	for _, stmt := range dc.stmtMap {
		err = dc.Access.Dm_build_389(stmt)
		if err != nil {
			return err
		}

		if stmt.paramCount > 0 {
			err = stmt.prepare()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (dc *DmConnection) close() error {
	if dc.closed.IsSet() {
		return nil
	}

	close(dc.closech)
	if dc.Access == nil {
		return nil
	}

	err := dc.rollback()
	if err != nil {
		return err
	}

	for _, stmt := range dc.stmtMap {
		err = stmt.free()
		if err != nil {
			return err
		}
	}

	if dc.stmtPool != nil {
		for _, spi := range dc.stmtPool {
			err = dc.Access.Dm_build_394(spi.id)
			if err != nil {
				return err
			}
		}
		dc.stmtPool = nil
	}

	err = dc.Access.Close()
	if err != nil {
		return err
	}

	dc.closed.Set(true)
	return nil
}

func (dc *DmConnection) ping(ctx context.Context) error {
	rows, err := dc.query("select 1", nil)
	if err != nil {
		return err
	}
	return rows.close()
}

func (dc *DmConnection) exec(query string, args []driver.Value) (*DmResult, error) {
	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	if args != nil && len(args) > 0 {
		stmt, err := dc.prepare(query)
		defer stmt.close()
		if err != nil {
			return nil, err
		}
		dc.lastExecInfo = stmt.execInfo

		return stmt.exec(args)
	} else {
		r1, err := dc.executeInner(query, Dm_build_686)
		if err != nil {
			return nil, err
		}

		if r2, ok := r1.(*DmResult); ok {
			return r2, nil
		} else {
			return nil, ECGO_NOT_EXEC_SQL.throw()
		}
	}
}

func (dc *DmConnection) execContext(ctx context.Context, query string, args []driver.NamedValue) (*DmResult, error) {

	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	if err := dc.watchCancel(ctx); err != nil {
		return nil, err
	}
	defer dc.finish()

	if args != nil && len(args) > 0 {
		stmt, err := dc.prepare(query)
		defer stmt.close()
		if err != nil {
			return nil, err
		}
		dc.lastExecInfo = stmt.execInfo

		return stmt.execContext(ctx, args)
	} else {
		r1, err := dc.executeInner(query, Dm_build_686)
		if err != nil {
			return nil, err
		}

		if r2, ok := r1.(*DmResult); ok {
			return r2, nil
		} else {
			return nil, ECGO_NOT_EXEC_SQL.throw()
		}
	}
}

func (dc *DmConnection) query(query string, args []driver.Value) (*DmRows, error) {

	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	if args != nil && len(args) > 0 {
		stmt, err := dc.prepare(query)
		if err != nil {
			stmt.close()
			return nil, err
		}
		dc.lastExecInfo = stmt.execInfo

		stmt.innerUsed = true
		return stmt.query(args)

	} else {
		r1, err := dc.executeInner(query, Dm_build_685)
		if err != nil {
			return nil, err
		}

		if r2, ok := r1.(*DmRows); ok {
			return r2, nil
		} else {
			return nil, ECGO_NOT_QUERY_SQL.throw()
		}
	}
}

func (dc *DmConnection) queryContext(ctx context.Context, query string, args []driver.NamedValue) (*DmRows, error) {

	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	if err := dc.watchCancel(ctx); err != nil {
		return nil, err
	}
	defer dc.finish()

	if args != nil && len(args) > 0 {
		stmt, err := dc.prepare(query)
		if err != nil {
			stmt.close()
			return nil, err
		}
		dc.lastExecInfo = stmt.execInfo

		stmt.innerUsed = true
		return stmt.queryContext(ctx, args)

	} else {
		r1, err := dc.executeInner(query, Dm_build_685)
		if err != nil {
			return nil, err
		}

		if r2, ok := r1.(*DmRows); ok {
			return r2, nil
		} else {
			return nil, ECGO_NOT_QUERY_SQL.throw()
		}
	}

}

func (dc *DmConnection) prepare(query string) (*DmStatement, error) {
	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	stmt, err := NewDmStmt(dc, query)
	if err != nil {
		return nil, err
	}

	err = stmt.prepare()
	return stmt, err
}

func (dc *DmConnection) prepareContext(ctx context.Context, query string) (*DmStatement, error) {
	err := dc.checkClosed()
	if err != nil {
		return nil, err
	}

	if err := dc.watchCancel(ctx); err != nil {
		return nil, err
	}
	defer dc.finish()

	stmt, err := dc.prepare(query)
	if err != nil {
		return nil, err
	}

	return stmt, nil
}

func (dc *DmConnection) resetSession(ctx context.Context) error {
	err := dc.checkClosed()
	if err != nil {
		return err
	}

	for _, stmt := range dc.stmtMap {
		stmt.inUse = false
	}

	return nil
}

func (dc *DmConnection) checkNamedValue(nv *driver.NamedValue) error {
	var err error
	var cvt = converter{dc, false}
	nv.Value, err = cvt.ConvertValue(nv.Value)
	dc.isBatch = cvt.isBatch
	return err
}

func (dc *DmConnection) driverQuery(query string) (*DmStatement, *DmRows, error) {
	stmt, err := NewDmStmt(dc, query)
	if err != nil {
		return nil, nil, err
	}
	stmt.innerUsed = true
	stmt.innerExec = true
	info, err := dc.Access.Dm_build_417(stmt, Dm_build_685)
	if err != nil {
		return nil, nil, err
	}
	dc.lastExecInfo = info
	stmt.innerExec = false
	return stmt, newDmRows(newInnerRows(0, stmt, info)), nil
}

func (dc *DmConnection) getIndexOnEPGroup() int32 {
	if dc.dmConnector.group == nil || dc.dmConnector.group.epList == nil {
		return -1
	}
	for i := 0; i < len(dc.dmConnector.group.epList); i++ {
		ep := dc.dmConnector.group.epList[i]
		if dc.dmConnector.host == ep.host && dc.dmConnector.port == ep.port {
			return int32(i)
		}
	}
	return -1
}

func (dc *DmConnection) getServerEncoding() string {
	if dc.dmConnector.charCode != "" {
		return dc.dmConnector.charCode
	}
	return dc.serverEncoding
}

func (dc *DmConnection) lobFetchAll() bool {
	return dc.dmConnector.lobMode == 2
}

func (conn *DmConnection) CompatibleOracle() bool {
	return conn.dmConnector.compatibleMode == COMPATIBLE_MODE_ORACLE
}

func (conn *DmConnection) CompatibleMysql() bool {
	return conn.dmConnector.compatibleMode == COMPATIBLE_MODE_MYSQL
}

func (conn *DmConnection) cancel(err error) {
	conn.canceled.Set(err)
	fmt.Println(conn.close())
}

func (conn *DmConnection) finish() {
	if !conn.watching || conn.finished == nil {
		return
	}
	select {
	case conn.finished <- struct{}{}:
		conn.watching = false
	case <-conn.closech:
	}
}

func (conn *DmConnection) startWatcher() {
	watcher := make(chan context.Context, 1)
	conn.watcher = watcher
	finished := make(chan struct{})
	conn.finished = finished
	go func() {
		for {
			var ctx context.Context
			select {
			case ctx = <-watcher:
			case <-conn.closech:
				return
			}

			select {
			case <-ctx.Done():
				conn.cancel(ctx.Err())
			case <-finished:
			case <-conn.closech:
				return
			}
		}
	}()
}

func (conn *DmConnection) watchCancel(ctx context.Context) error {
	if conn.watching {

		return conn.close()
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if ctx.Done() == nil {
		return nil
	}

	if conn.watcher == nil {
		return nil
	}

	conn.watching = true
	conn.watcher <- ctx
	return nil
}

type noCopy struct{}

func (*noCopy) Lock() {}

type atomicBool struct {
	_noCopy noCopy
	value   uint32
}

func (ab *atomicBool) IsSet() bool {
	return atomic.LoadUint32(&ab.value) > 0
}

func (ab *atomicBool) Set(value bool) {
	if value {
		atomic.StoreUint32(&ab.value, 1)
	} else {
		atomic.StoreUint32(&ab.value, 0)
	}
}

func (ab *atomicBool) TrySet(value bool) bool {
	if value {
		return atomic.SwapUint32(&ab.value, 1) == 0
	}
	return atomic.SwapUint32(&ab.value, 0) > 0
}

type atomicError struct {
	_noCopy noCopy
	value   atomic.Value
}

func (ae *atomicError) Set(value error) {
	ae.value.Store(value)
}

func (ae *atomicError) Value() error {
	if v := ae.value.Load(); v != nil {

		return v.(error)
	}
	return nil
}
