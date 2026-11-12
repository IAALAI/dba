package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var (
	errBadID    = errors.New("无效的 id")
	errNotFound = errors.New("没有这个监控目标")
)

type itemDoc struct {
	ID   string
	Name string
	Type int16
	DSN  string
}

func (it itemDoc) rawDSN() string {
	return strings.TrimSpace(it.DSN)
}

type itemView struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     int16    `json:"type"`
	TypeName string   `json:"type_name"`
	Endpoint string   `json:"endpoint"`
	Options  []string `json:"options"`
	Status   string   `json:"status"`
	Bytes    int64    `json:"bytes"`
	LastAt   int64    `json:"last_at"`
	Error    string   `json:"error"`
}

type itemDetail struct {
	itemView
	DSN string `json:"dsn"`
}

type statView struct {
	At     int64  `json:"at"`
	Online bool   `json:"online"`
	Bytes  int64  `json:"bytes"`
	Error  string `json:"error"`
}

type itemInput struct {
	Name string `json:"name"`
	DSN  string `json:"dsn"`
}

type App struct {
	store   *StatsStore
	probeMu chan struct{}
	loginMu *attemptLimiter
}

func newApp(store *StatsStore) *App {
	return &App{
		store:   store,
		probeMu: make(chan struct{}, 1),
		loginMu: newAttemptLimiter(),
	}
}

func (a *App) mount(r *gin.Engine) {
	r.POST("/api/login", a.login)
	api := r.Group("/api", a.authMiddleware())
	api.POST("/logout", a.logout)
	api.POST("/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "success", "message": "ok"})
	})
	api.GET("/list", a.list)
	api.POST("/item", a.create)
	api.GET("/item/:id", a.get)
	api.PUT("/item/:id", a.update)
	api.DELETE("/item/:id", a.remove)
	api.GET("/item/:id/stats", a.history)
	api.POST("/item/:id/probe", a.probe)
	api.POST("/refresh", a.refresh)
}

func (a *App) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c.GetHeader("Authorization"))
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		ok, err := a.store.ValidSession(ctx, token, time.Now())
		if err != nil {
			writeErr(c, err)
			c.Abort()
			return
		}
		if !ok {
			fail(c, http.StatusUnauthorized, "登录已失效")
			c.Abort()
			return
		}
		c.Next()
	}
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

type loginInput struct {
	AccessKey string `json:"access_key"`
	Code      string `json:"code"`
}

func (a *App) login(c *gin.Context) {
	var in loginInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "请求体需要是 JSON")
		return
	}
	in.AccessKey = strings.TrimSpace(in.AccessKey)
	in.Code = strings.ReplaceAll(strings.TrimSpace(in.Code), " ", "")
	if in.AccessKey == "" || in.Code == "" {
		fail(c, http.StatusBadRequest, "请填写 Access Key 和验证码")
		return
	}
	if len(in.AccessKey) > 200 || len(in.Code) > 8 {
		fail(c, http.StatusBadRequest, "Access Key 或验证码不正确")
		return
	}
	ip := c.ClientIP()
	now := time.Now()
	if a.loginMu.blocked(ip, now) {
		fail(c, http.StatusTooManyRequests, "尝试过多，请稍后再试")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, expires, err := a.store.Login(ctx, in.AccessKey, in.Code, now)
	if errors.Is(err, errAuthFailed) {
		a.loginMu.fail(ip, now)
		fail(c, http.StatusUnauthorized, "Access Key 或验证码不正确")
		return
	}
	if err != nil {
		writeErr(c, err)
		return
	}
	a.loginMu.success(ip)
	c.JSON(http.StatusOK, gin.H{"token": token, "expires_at": expires})
}

func (a *App) logout(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := a.store.Logout(ctx, bearerToken(c.GetHeader("Authorization"))); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "ok"})
}

func fail(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{"status": "error", "message": message})
}

func writeErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errBadID):
		fail(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, errNotFound):
		fail(c, http.StatusNotFound, err.Error())
	default:
		fail(c, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	items, err := a.allItems(ctx)
	if err != nil {
		writeErr(c, err)
		return
	}
	views, err := a.views(ctx, items)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, views)
}

func (a *App) create(c *gin.Context) {
	var in itemInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "请求体需要是 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.DSN = strings.TrimSpace(in.DSN)
	if in.Name == "" || in.DSN == "" {
		fail(c, http.StatusBadRequest, "名称和连接串都不能为空")
		return
	}
	if len(in.Name) > 80 || len(in.DSN) > 4000 {
		fail(c, http.StatusBadRequest, "名称或连接串过长")
		return
	}
	tgt, err := ParseTarget(in.DSN)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	doc, err := a.store.InsertTarget(ctx, itemDoc{Name: in.Name, Type: tgt.Kind, DSN: in.DSN})
	if err != nil {
		writeErr(c, err)
		return
	}
	view, err := a.probeAndSave(ctx, doc)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (a *App) get(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	it, err := a.findItem(ctx, c.Param("id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	views, err := a.views(ctx, []itemDoc{it})
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, itemDetail{itemView: views[0], DSN: it.rawDSN()})
}

func (a *App) update(c *gin.Context) {
	var in itemInput
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "请求体需要是 JSON")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.DSN = strings.TrimSpace(in.DSN)
	if in.Name == "" {
		fail(c, http.StatusBadRequest, "名称不能为空")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	it, err := a.findItem(ctx, c.Param("id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	dsn := it.rawDSN()
	kind := it.Type
	if in.DSN != "" {
		tgt, err := ParseTarget(in.DSN)
		if err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		dsn = in.DSN
		kind = tgt.Kind
	}
	it.Name = in.Name
	it.Type = kind
	it.DSN = dsn
	if err := a.store.UpdateTarget(ctx, it); err != nil {
		writeErr(c, err)
		return
	}
	view, err := a.probeAndSave(ctx, it)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, itemDetail{itemView: view, DSN: dsn})
}

func (a *App) remove(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	it, err := a.findItem(ctx, c.Param("id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	if err := a.store.DeleteTarget(ctx, it.ID); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "ok"})
}

func (a *App) history(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	it, err := a.findItem(ctx, c.Param("id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	limit := 168
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	rows, err := a.store.History(ctx, it.ID, limit)
	if err != nil {
		writeErr(c, err)
		return
	}
	out := make([]statView, 0, len(rows))
	for _, row := range rows {
		out = append(out, statView{At: row.At, Online: row.Online, Bytes: row.Bytes, Error: row.Error})
	}
	c.JSON(http.StatusOK, out)
}

func (a *App) probe(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	it, err := a.findItem(ctx, c.Param("id"))
	if err != nil {
		writeErr(c, err)
		return
	}
	view, err := a.probeAndSave(ctx, it)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (a *App) refresh(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Minute)
	defer cancel()
	views, err := a.probeAll(ctx)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, views)
}

func (a *App) allItems(ctx context.Context) ([]itemDoc, error) {
	return a.store.ListTargets(ctx)
}

func (a *App) findItem(ctx context.Context, id string) (itemDoc, error) {
	if len(id) != 32 {
		return itemDoc{}, errBadID
	}
	return a.store.GetTarget(ctx, id)
}

func (a *App) views(ctx context.Context, items []itemDoc) ([]itemView, error) {
	latest, err := a.store.Latest(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]itemView, 0, len(items))
	for _, it := range items {
		view, _, _ := describe(it)
		if stat, ok := latest[it.ID]; ok {
			applyStat(&view, stat)
		}
		out = append(out, view)
	}
	return out, nil
}

func (a *App) probeAll(ctx context.Context) ([]itemView, error) {
	if err := a.lockProbe(ctx); err != nil {
		return nil, err
	}
	defer a.unlockProbe()
	items, err := a.allItems(ctx)
	if err != nil {
		return nil, err
	}
	views := make([]itemView, 0, len(items))
	for _, it := range items {
		view, err := a.storeSample(ctx, it)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (a *App) probeAndSave(ctx context.Context, it itemDoc) (itemView, error) {
	if err := a.lockProbe(ctx); err != nil {
		return itemView{}, err
	}
	defer a.unlockProbe()
	return a.storeSample(ctx, it)
}

func (a *App) lockProbe(ctx context.Context) error {
	select {
	case a.probeMu <- struct{}{}:
		return nil
	case <-ctx.Done():
		return errors.New("已有采集任务在进行")
	}
}

func (a *App) unlockProbe() {
	select {
	case <-a.probeMu:
	default:
	}
}

func (a *App) storeSample(ctx context.Context, it itemDoc) (itemView, error) {
	view, tgt, parseErr := describe(it)
	stat := Stat{ItemID: it.ID, At: time.Now().Unix()}
	if parseErr != nil {
		stat.Error = parseErr.Error()
	} else {
		result := tgt.Sample(ctx)
		stat.Online = result.Online
		stat.Bytes = result.Bytes
		stat.Error = result.Error
	}
	if err := a.store.Insert(ctx, stat); err != nil {
		return view, err
	}
	applyStat(&view, stat)
	return view, nil
}

func describe(it itemDoc) (itemView, *Target, error) {
	view := itemView{
		ID:       it.ID,
		Name:     it.Name,
		Type:     it.Type,
		TypeName: kindLabel(it.Type),
		Endpoint: "未配置",
		Options:  []string{},
		Status:   "unknown",
	}
	raw := it.rawDSN()
	if raw == "" {
		return view, nil, errors.New("缺少连接串，请改成 mysql:// 或 postgres://")
	}
	tgt, err := ParseTarget(raw)
	if err != nil {
		view.Endpoint = "无法解析"
		return view, nil, err
	}
	view.Type = tgt.Kind
	view.TypeName = tgt.KindName
	view.Endpoint = tgt.Endpoint
	view.Options = tgt.Options
	return view, tgt, nil
}

func applyStat(view *itemView, stat Stat) {
	view.LastAt = stat.At
	view.Bytes = stat.Bytes
	view.Error = stat.Error
	if stat.Online {
		view.Status = "online"
	} else {
		view.Status = "offline"
	}
}

func kindLabel(kind int16) string {
	switch kind {
	case kindMySQL:
		return "mysql"
	case kindPostgres:
		return "postgres"
	default:
		return "unknown"
	}
}
