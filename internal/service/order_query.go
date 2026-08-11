// internal/service/order_query.go
package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/example/epay-go/internal/config"
	"github.com/example/epay-go/internal/model"
	"github.com/example/epay-go/internal/payment"
	"github.com/example/epay-go/internal/repository"
	"github.com/example/epay-go/internal/worker"
)

const queryClaimBatchSize = 50

// QueryIntervals 主动查单相邻两次查询的间隔（累加）：创建后20s第1次，之后依次再等30s/60s/120s/300s
var QueryIntervals = []time.Duration{
	20 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
	300 * time.Second,
}

// FirstQueryAt 计算订单创建时应写入的首次主动查单时间
func FirstQueryAt(from time.Time) time.Time {
	return from.Add(QueryIntervals[0])
}

// OrderQueryService 订单主动查单补偿服务
type OrderQueryService struct {
	orderRepo    *repository.OrderRepository
	channelRepo  *repository.ChannelRepository
	orderSvc     *OrderService
	notifySvc    *NotifyService
	concurrency  int
	pollInterval time.Duration
	pool         *worker.Pool
}

var (
	orderQueryOnce sync.Once
	orderQueryInst *OrderQueryService
)

func NewOrderQueryService() *OrderQueryService {
	orderQueryOnce.Do(func() {
		concurrency := 16
		pollSec := 2
		if cfg := config.Get(); cfg != nil {
			concurrency = cfg.Worker.QueryConcurrency
			pollSec = cfg.Worker.QueryPollIntervalSec
		}
		orderQueryInst = &OrderQueryService{
			orderRepo:    repository.NewOrderRepository(),
			channelRepo:  repository.NewChannelRepository(),
			orderSvc:     NewOrderService(),
			notifySvc:    NewNotifyService(),
			concurrency:  concurrency,
			pollInterval: time.Duration(pollSec) * time.Second,
		}
	})
	return orderQueryInst
}

// StartQueryWorker 启动主动查单调度 + 固定并发执行池
func (s *OrderQueryService) StartQueryWorker(ctx context.Context) {
	s.pool = worker.NewPool(s.concurrency)
	s.pool.Start(ctx)

	log.Printf("Order query worker started: concurrency=%d poll=%s", s.concurrency, s.pollInterval)

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Order query worker stopped")
			s.pool.Wait()
			return
		case <-ticker.C:
			s.dispatchQuery(ctx)
		}
	}
}

// dispatchQuery 认领到期查单并投入固定并发池
func (s *OrderQueryService) dispatchQuery(ctx context.Context) {
	orders, err := s.orderRepo.ClaimPendingQueryOrders(queryClaimBatchSize)
	if err != nil {
		log.Printf("Claim pending query orders failed: %v", err)
		return
	}
	if len(orders) == 0 {
		return
	}

	for i := range orders {
		order := orders[i]
		ok := s.pool.Submit(ctx, func(context.Context) {
			s.queryAndProcess(&order)
		})
		if !ok {
			return
		}
	}
}

// queryAndProcess 主动查询单个订单的上游状态并处理
func (s *OrderQueryService) queryAndProcess(order *model.Order) {
	channel, err := s.channelRepo.GetByID(order.ChannelID)
	if err != nil {
		log.Printf("Active query: channel not found trade_no=%s: %v", order.TradeNo, err)
		s.scheduleNext(order)
		return
	}

	adapter, err := payment.NewAdapter(channel.Plugin, channel.Config)
	if err != nil {
		log.Printf("Active query: create adapter failed trade_no=%s: %v", order.TradeNo, err)
		s.scheduleNext(order)
		return
	}

	resp, err := adapter.QueryOrder(context.Background(), order.TradeNo)
	if err != nil {
		log.Printf("Active query failed trade_no=%s: %v", order.TradeNo, err)
		s.scheduleNext(order)
		return
	}

	log.Printf("Active query result: trade_no=%s status=%s", order.TradeNo, resp.Status)

	switch resp.Status {
	case "paid":
		if err := s.orderSvc.ProcessPayNotify(order.TradeNo, resp.ApiTradeNo, "", resp.Amount); err != nil {
			log.Printf("Active query: process pay notify failed trade_no=%s: %v", order.TradeNo, err)
			s.scheduleNext(order)
			return
		}
		if err := s.orderRepo.UpdateQueryStatus(order.TradeNo, nil); err != nil {
			log.Printf("Active query: update query status failed trade_no=%s: %v", order.TradeNo, err)
		}
		if paidOrder, err := s.orderSvc.GetByTradeNo(order.TradeNo); err == nil && paidOrder.Status == model.OrderStatusPaid {
			s.notifySvc.Wake()
		}
	case "closed":
		if err := s.orderRepo.UpdateQueryStatus(order.TradeNo, nil); err != nil {
			log.Printf("Active query: update query status failed trade_no=%s: %v", order.TradeNo, err)
		}
	default:
		s.scheduleNext(order)
	}
}

// scheduleNext 按累加间隔安排下一次查询，超出重试次数则终止调度
func (s *OrderQueryService) scheduleNext(order *model.Order) {
	nextIdx := order.QueryCount + 1
	var nextAt *time.Time
	if nextIdx < len(QueryIntervals) {
		t := time.Now().Add(QueryIntervals[nextIdx])
		nextAt = &t
	}
	if err := s.orderRepo.UpdateQueryStatus(order.TradeNo, nextAt); err != nil {
		log.Printf("Update query status failed trade_no=%s: %v", order.TradeNo, err)
	}
}
