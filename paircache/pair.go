package paircache

import (
	"encoding/json"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/gopool"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/paircache/mysqldb"
	"github.com/ethereum/go-ethereum/paircache/pairtypes"
	"github.com/jmoiron/sqlx"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	TriangleChannel   = make(chan *pairtypes.TransferTriangle)
	DoneChannel       = make(chan struct{})
	pairCache         = pairtypes.NewPairCache()
	ABI               *abi.ABI
	AbiStr            string
	From              common.Address
	To                common.Address
	ToStr             string
	ClusterId         int64
	ClusterTotal      int64
	ConfigItemUrl     string
	ChainId           int64
	Type              string
	MevServiceUrl     string
	TrianglefilterNum int
	PairCallTimeout   int
	PairCallDeadline  int64
	PairCallSwitch    bool
	ProfitThreshold   int64
	EsGasLimit        uint64
)

// 处理通道中的数据
func ProcessTriangle(pairAPI pairtypes.PairAPI) {
	for {
		select {
		case transferTriangle := <-TriangleChannel:
			if deadline := IsOutPairCallDeadline(transferTriangle.BlockTime, "去重随机获取triangles个数="+strconv.Itoa(len(transferTriangle.Triangles))); !deadline {
				pairAPI.PairCallBatch(transferTriangle)
				// 处理完成通知
				// paircache.DoneChannel <- struct{}{}
			}
		}
	}
}

func InitPairCache() {
	// 初始化triange到内存
	printMemUsed()
	fetchTriangleMap()
	printMemUsed()

	// 初始化topic到内存
	fetchDynamicConfig()

	// 开启协程周期更新内存中triange与topic
	err := gopool.Submit(timerGetTriangle)
	if err != nil {
		log.Error("开启定时加载Triangle任务失败", "err", err)
		return
	}
	err = gopool.Submit(timerGetDynamicConfig)
	if err != nil {
		log.Error("开启定时加载动态配置任务失败", "err", err)
		return
	}

	// 加载三角合约abi
	if parsed, err := abi.JSON(strings.NewReader(AbiStr)); err != nil {
		log.Error("加载三角合约abi失败", "err", err)
		return
	} else {
		ABI = &parsed
	}
	log.Info("加载三角合约abi到内存中成功", "AbiStr", AbiStr, "ABI", *ABI)

	// printCacheToFile()
}

func printCacheToFile() {
	createFile := func(filePath string, cache any) {
		// 创建文件
		file, err := os.Create(filePath)
		if err != nil {
			return
		}
		defer file.Close()

		// 将 map 编码为 JSON
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ") // 设置缩进格式
		if err := encoder.Encode(cache); err != nil {
			return
		}
		log.Info("结果输出到文件完成，结束")
	}

	topicMap := pairCache.TopicMap
	createFile("/bc/topic.json", topicMap)

	triangleMap := pairCache.TriangleMap
	createFile("/bc/triangle.json", triangleMap)

	pairTriangleIdSetMap := pairCache.PairTriangleIdSetMap
	m := make(map[string][]string)
	for tuple := range pairTriangleIdSetMap.IterBuffered() {
		set := tuple.Val.(*pairtypes.Set)
		data := set.GetData()
		m[tuple.Key] = data.Keys()
	}
	createFile("/bc/pairTriangleIdSet.json", m)

}

func GetPairCache() *pairtypes.PairCache {
	return pairCache
}

func timerGetTriangle() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fetchTriangleMap()
		}
	}
}

func timerGetDynamicConfig() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fetchDynamicConfig()
		}
	}
}

func fetchDynamicConfig() {
	// 发送GET请求，获取最新的配置信息
	start := time.Now()
	resp, err := http.Get(ConfigItemUrl)
	if err != nil {
		log.Error("http请求配置url失败", "err", err)
		return
	}
	defer resp.Body.Close() // 确保函数结束时关闭响应体

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取http请求响应配置数据失败", "err", err)
		return
	}

	// 定义用于存储解析后的数据的 map
	var result map[string]interface{}

	// 解析 JSON 数据
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Error("解析配置数据失败", "err", err)
		return
	}

	// 打印解析后的结果
	// log.Info("解析配置数据成功", "config-item", result)

	// 获取特定的键值对
	if isOpen, ok := result["open"].(bool); ok {
		PairCallSwitch = isOpen
		// log.Info("刷新内存中pairCallSwitch成功", "pairCallSwitch", PairCallSwitch)
	}

	if triangleCount, ok := result["triangleCount"].(float64); ok {
		TrianglefilterNum = int(triangleCount)
		// log.Info("刷新内存中TrianglefilterNum成功", "trianglefilterNum", TrianglefilterNum)
	}

	if threadTtl, ok := result["threadTtl"].(float64); ok {
		PairCallTimeout = int(threadTtl)
		// log.Info("刷新内存中PairCallTimeout成功", "pairCallTimeout", PairCallTimeout)
	}

	if pairCallDeadline, ok := result["deadline"].(float64); ok {
		PairCallDeadline = int64(pairCallDeadline)
		// log.Info("刷新内存中PairCallDeadline成功", "pairCallDeadline", PairCallDeadline)
	}

	if mevServiceUrl, ok := result["mevServiceUrl"].(string); ok {
		MevServiceUrl = mevServiceUrl
		// log.Info("刷新内存中mevServiceUrl成功", "mevServiceUrl", MevServiceUrl)
	}

	if profitThreshold, ok := result["profitThreshold"].(float64); ok {
		ProfitThreshold = int64(profitThreshold)
		// log.Info("刷新内存中profitThreshold成功", "profitThreshold", ProfitThreshold)
	}

	if contracts, ok := result["contracts"].(string); ok {
		ToStr = contracts
		To = common.HexToAddress(contracts)
		// log.Info("刷新内存中三角合约地址成功", "contracts address", To)
	}

	if topics, ok := result["topics"].(map[string]interface{}); ok {
		newTopicMap := make(map[string]string)
		for key, value := range topics {
			newTopicMap[key] = value.(string)
		}
		pairCache.TopicMap = newTopicMap
		// log.Info("刷新内存中topic成功", "topic总数", len(newTopicMap))
	}

	log.Info("刷新动态配置完成", "time", time.Since(start))
}

func fetchTriangleMap() {
	// 初始化数据库连接
	start := time.Now()
	mysqlDB := mysqldb.GetMysqlDB()

	// 使用流式查询，逐行处理数据
	rows, err := mysqlDB.Queryx("select id, token0, router0, pair0, token1, router1, pair1, token2, router2, pair2 from arbitrage_triangle order by id")
	if err != nil {
		log.Error("查询失败", "err", err)
	}
	defer func(rows *sqlx.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error("流式查询关闭rows失败", "err", err)
		}
	}(rows)

	// 遍历查询结果
	for rows.Next() {
		triangle := pairtypes.Triangle{}
		err := rows.StructScan(&triangle)
		if err != nil {
			log.Error("填充结果到结构体失败", "err", err)
		}

		// 集群中单个节点获取分配的triangle，通过取余方式分组，余数范围：[0,ClusterTotal-1]，而ClusterId范围：[1,ClusterTotal]
		if triangle.ID%ClusterTotal == ClusterId-1 {
			id := strconv.FormatInt(triangle.ID, 10)
			triangle.Pair0 = common.HexToAddress(triangle.Pair0).Hex()
			triangle.Pair1 = common.HexToAddress(triangle.Pair1).Hex()
			triangle.Pair2 = common.HexToAddress(triangle.Pair2).Hex()
			pairCache.AddTriangle(id, triangle)
			pairCache.AddPairTriangleId(triangle.Pair0, id)
			pairCache.AddPairTriangleId(triangle.Pair1, id)
			pairCache.AddPairTriangleId(triangle.Pair2, id)
		}
	}

	// 检查是否有遍历中的错误
	if err := rows.Err(); err != nil {
		log.Error("查询失败", "err", err)
	}
	log.Info("刷新内存中triange耗时", "time", time.Since(start), "triange总数", pairCache.TriangleMapSize(), "pair总数", pairCache.PairTriangleIdSetMapSize(), "集群id", ClusterId, "集群总数", ClusterTotal)
}

func printMemUsed() {
	// 读取 /proc/meminfo 文件
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Error("读取内存文件/proc/meminfo失败", "err", err)
		return
	}

	// 解析内容
	lines := strings.Split(string(data), "\n")
	memInfo := make(map[string]int64)

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.Trim(fields[0], ":")
		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err == nil {
			memInfo[key] = value
		}
	}

	// 计算总缓存内存
	totalCache := memInfo["Buffers"] + memInfo["Cached"]

	// 输出总内存、空闲内存、可用内存和总缓存内存
	log.Info("Total RAM (MB)", "MemTotal", memInfo["MemTotal"]/1024)
	log.Info("Free RAM (MB)", "MemFree", memInfo["MemFree"]/1024)
	log.Info("Available RAM (MB)", "MemAvailable", memInfo["MemAvailable"]/1024)
	log.Info("Total Cached RAM (Buffers + Cached) (MB)", "Buffers + Cached", totalCache/1024)
}

func Encoder(name string, args ...interface{}) ([]byte, error) {
	return ABI.Pack(name, args...)
}

func IsOutPairCallDeadline(blockTime *time.Time, desc string) bool {
	timeDiff := time.Now().Sub(*blockTime).Milliseconds()
	log.Info(desc, "与区块生成时间间隔ms", timeDiff)
	return timeDiff > PairCallDeadline
}

func SelectRandomElements(slice []pairtypes.Triangle, count int) []pairtypes.Triangle {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(slice) - 1; i > len(slice)-1-count; i-- {
		j := r.Intn(i + 1)                      // 随机选择范围逐渐缩小
		slice[i], slice[j] = slice[j], slice[i] // 每次随机选择一个元素，并将其与最后一个元素
	}
	return slice[len(slice)-count:]
}
