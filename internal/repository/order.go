// internal/repository/order.go
package repository

import (
	"time"

	"github.com/example/epay-go/internal/database"
	"github.com/example/epay-go/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 认领租约：处理中把 next_*_at 推到未来，避免被重复捞取；崩溃后到期可再认领
const pendingClaimLease = 60 * time.Second

type OrderRepository struct {
	db *gorm.DB
}

func NewOrderRepository() *OrderRepository {
	return &OrderRepository{db: database.Get()}
}

// Create 创建订单
func (r *OrderRepository) Create(order *model.Order) error {
	return r.db.Create(order).Error
}

// GetByID 根据ID获取订单
func (r *OrderRepository) GetByID(id int64) (*model.Order, error) {
	var order model.Order
	err := r.db.Preload("Merchant").Preload("Channel").First(&order, id).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// GetByTradeNo 根据系统订单号获取订单
func (r *OrderRepository) GetByTradeNo(tradeNo string) (*model.Order, error) {
	var order model.Order
	err := r.db.Preload("Merchant").Preload("Channel").
		Where("trade_no = ?", tradeNo).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// GetByOutTradeNo 根据商户订单号获取订单
func (r *OrderRepository) GetByOutTradeNo(merchantID int64, outTradeNo string) (*model.Order, error) {
	var order model.Order
	err := r.db.Where("merchant_id = ? AND out_trade_no = ?", merchantID, outTradeNo).
		First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// Update 更新订单
func (r *OrderRepository) Update(order *model.Order) error {
	return r.db.Save(order).Error
}

// UpdateStatus 更新订单状态
func (r *OrderRepository) UpdateStatus(tradeNo string, status int8) error {
	updates := map[string]interface{}{"status": status}
	if status == model.OrderStatusPaid {
		now := time.Now()
		updates["paid_at"] = &now
	}
	return r.db.Model(&model.Order{}).Where("trade_no = ?", tradeNo).Updates(updates).Error
}

// UpdateNotifyStatus 更新通知状态
func (r *OrderRepository) UpdateNotifyStatus(tradeNo string, status int8, nextNotifyAt *time.Time) error {
	updates := map[string]interface{}{
		"notify_status": status,
		"notify_count":  gorm.Expr("notify_count + 1"),
	}
	if nextNotifyAt != nil {
		updates["next_notify_at"] = nextNotifyAt
	}
	return r.db.Model(&model.Order{}).Where("trade_no = ?", tradeNo).Updates(updates).Error
}

// UpdatePayInfo 更新支付信息
func (r *OrderRepository) UpdatePayInfo(tradeNo, apiTradeNo, buyer string) error {
	now := time.Now()
	return r.db.Model(&model.Order{}).Where("trade_no = ?", tradeNo).Updates(map[string]interface{}{
		"api_trade_no": apiTradeNo,
		"buyer":        buyer,
		"status":       model.OrderStatusPaid,
		"paid_at":      &now,
	}).Error
}

// List 分页查询订单列表
func (r *OrderRepository) List(page, pageSize int, merchantID *int64, status *int8) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	query := r.db.Model(&model.Order{})
	if merchantID != nil {
		query = query.Where("merchant_id = ?", *merchantID)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Preload("Merchant").Preload("Channel").
		Offset(offset).Limit(pageSize).Order("id DESC").Find(&orders).Error
	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// ClaimPendingNotifyOrders 认领待通知订单（FOR UPDATE SKIP LOCKED + 短租约）
func (r *OrderRepository) ClaimPendingNotifyOrders(limit int) ([]model.Order, error) {
	var claimed []model.Order
	now := time.Now()
	leaseUntil := now.Add(pendingClaimLease)

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var orders []model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND notify_status < ? AND (next_notify_at IS NULL OR next_notify_at <= ?)",
				model.OrderStatusPaid, model.NotifyStatusSuccess, now).
			Order("id ASC").
			Limit(limit).
			Find(&orders).Error; err != nil {
			return err
		}
		if len(orders) == 0 {
			return nil
		}

		ids := make([]int64, len(orders))
		for i := range orders {
			ids[i] = orders[i].ID
		}
		if err := tx.Model(&model.Order{}).Where("id IN ?", ids).
			Updates(map[string]interface{}{
				"notify_status":  model.NotifyStatusSending,
				"next_notify_at": leaseUntil,
			}).Error; err != nil {
			return err
		}

		for i := range orders {
			orders[i].NotifyStatus = model.NotifyStatusSending
			t := leaseUntil
			orders[i].NextNotifyAt = &t
		}
		claimed = orders
		return nil
	})
	return claimed, err
}

// ClaimPendingQueryOrders 认领待主动查询订单（FOR UPDATE SKIP LOCKED + 短租约）
func (r *OrderRepository) ClaimPendingQueryOrders(limit int) ([]model.Order, error) {
	var claimed []model.Order
	now := time.Now()
	leaseUntil := now.Add(pendingClaimLease)

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var orders []model.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND next_query_at IS NOT NULL AND next_query_at <= ?",
				model.OrderStatusUnpaid, now).
			Order("id ASC").
			Limit(limit).
			Find(&orders).Error; err != nil {
			return err
		}
		if len(orders) == 0 {
			return nil
		}

		ids := make([]int64, len(orders))
		for i := range orders {
			ids[i] = orders[i].ID
		}
		if err := tx.Model(&model.Order{}).Where("id IN ?", ids).
			Update("next_query_at", leaseUntil).Error; err != nil {
			return err
		}

		for i := range orders {
			t := leaseUntil
			orders[i].NextQueryAt = &t
		}
		claimed = orders
		return nil
	})
	return claimed, err
}

// GetPendingNotifyOrders 获取待通知的订单（只读，不做认领；测试/运维用）
func (r *OrderRepository) GetPendingNotifyOrders(limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Where("status = ? AND notify_status < ? AND (next_notify_at IS NULL OR next_notify_at <= ?)",
		model.OrderStatusPaid, model.NotifyStatusSuccess, time.Now()).
		Limit(limit).Find(&orders).Error
	return orders, err
}

// GetPendingQueryOrders 获取待主动查询的未支付订单（只读，不做认领）
func (r *OrderRepository) GetPendingQueryOrders(limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db.Where("status = ? AND next_query_at IS NOT NULL AND next_query_at <= ?",
		model.OrderStatusUnpaid, time.Now()).
		Limit(limit).Find(&orders).Error
	return orders, err
}

// UpdateQueryStatus 更新主动查询进度
func (r *OrderRepository) UpdateQueryStatus(tradeNo string, nextQueryAt *time.Time) error {
	updates := map[string]interface{}{"query_count": gorm.Expr("query_count + 1")}
	if nextQueryAt != nil {
		updates["next_query_at"] = nextQueryAt
	} else {
		updates["next_query_at"] = gorm.Expr("NULL")
	}
	return r.db.Model(&model.Order{}).Where("trade_no = ?", tradeNo).Updates(updates).Error
}

// GetTodayStats 获取今日统计
func (r *OrderRepository) GetTodayStats(merchantID *int64) (int64, decimal.Decimal, error) {
	var count int64

	today := time.Now().Format("2006-01-02")
	query := r.db.Model(&model.Order{}).
		Where("status = ? AND DATE(created_at) = ?", model.OrderStatusPaid, today)

	if merchantID != nil {
		query = query.Where("merchant_id = ?", *merchantID)
	}

	err := query.Count(&count).Error
	if err != nil {
		return 0, decimal.Zero, err
	}

	var result struct {
		Total decimal.Decimal
	}
	err = query.Select("COALESCE(SUM(amount), 0) as total").Scan(&result).Error
	if err != nil {
		return 0, decimal.Zero, err
	}

	return count, result.Total, nil
}
