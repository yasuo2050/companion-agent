// Package mongo 实现 Mock Business Service 的 MongoDB 持久化。
package mongo

import (
	"context"
	"time"

	"companion-agent/services/mock-business-service/internal/business"
	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Repository 管理 companion_agent_mvp 数据库中的业务集合。
type Repository struct {
	client *driver.Client
	db     *driver.Database
}

// New 创建连接并检查 MongoDB 可用性。
func New(ctx context.Context, uri, database string) (*Repository, error) {
	client, err := driver.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	return &Repository{client: client, db: client.Database(database)}, nil
}

// Close 关闭 MongoDB 客户端。
func (r *Repository) Close(ctx context.Context) error { return r.client.Disconnect(ctx) }

// Ping 检查 MongoDB 连接。
func (r *Repository) Ping(ctx context.Context) error { return r.client.Ping(ctx, nil) }

// EnsureIndexes 创建交易不变量所需索引。
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	orders := r.db.Collection("orders")
	drafts := r.db.Collection("order_drafts")
	companions := r.db.Collection("companions")
	if _, err := orders.Indexes().CreateMany(ctx, []driver.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "draft_id", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"draft_id": bson.M{"$exists": true}})},
		{Keys: bson.D{{Key: "companion_id", Value: 1}, {Key: "status", Value: 1}, {Key: "start_time", Value: 1}, {Key: "end_time", Value: 1}}},
	}); err != nil {
		return err
	}
	if _, err := drafts.Indexes().CreateMany(ctx, []driver.IndexModel{
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "conversation_id", Value: 1}, {Key: "status", Value: 1}, {Key: "updated_at", Value: -1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "conversation_id", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"status": business.DraftActive})},
	}); err != nil {
		return err
	}
	_, err := companions.Indexes().CreateOne(ctx, driver.IndexModel{Keys: bson.D{{Key: "game_ids", Value: 1}, {Key: "gender", Value: 1}, {Key: "price_per_hour", Value: 1}}})
	return err
}

func (r *Repository) GetProfile(ctx context.Context, id string) (business.UserProfile, error) {
	var value business.UserProfile
	err := r.db.Collection("user_profiles").FindOne(ctx, bson.M{"_id": id}).Decode(&value)
	return value, err
}
func (r *Repository) RecentOrders(ctx context.Context, userID string, limit int32) ([]business.Order, error) {
	cur, err := r.db.Collection("orders").Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var values []business.Order
	err = cur.All(ctx, &values)
	return values, err
}
func (r *Repository) ListCompanions(ctx context.Context) ([]business.Companion, error) {
	cur, err := r.db.Collection("companions").Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var values []business.Companion
	err = cur.All(ctx, &values)
	return values, err
}
func (r *Repository) GetCompanion(ctx context.Context, id string) (business.Companion, error) {
	var value business.Companion
	err := r.db.Collection("companions").FindOne(ctx, bson.M{"_id": id}).Decode(&value)
	return value, err
}
func (r *Repository) HasConfirmedOverlap(ctx context.Context, id string, start, end time.Time) (bool, error) {
	count, err := r.db.Collection("orders").CountDocuments(ctx, bson.M{"companion_id": id, "status": business.OrderConfirmed, "start_time": bson.M{"$lt": end}, "end_time": bson.M{"$gt": start}})
	return count > 0, err
}
func (r *Repository) SupersedeActiveDrafts(ctx context.Context, userID, conversationID string, now time.Time) error {
	_, err := r.db.Collection("order_drafts").UpdateMany(ctx, bson.M{"user_id": userID, "conversation_id": conversationID, "status": business.DraftActive}, bson.M{"$set": bson.M{"status": business.DraftSuperseded, "updated_at": now}})
	return err
}
func (r *Repository) InsertDraft(ctx context.Context, value business.Draft) error {
	_, err := r.db.Collection("order_drafts").InsertOne(ctx, value)
	return err
}
func (r *Repository) GetDraft(ctx context.Context, id string) (business.Draft, error) {
	var value business.Draft
	err := r.db.Collection("order_drafts").FindOne(ctx, bson.M{"_id": id}).Decode(&value)
	return value, err
}
func (r *Repository) SetDraftStatus(ctx context.Context, id, status string, now time.Time) error {
	_, err := r.db.Collection("order_drafts").UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"status": status, "updated_at": now}})
	return err
}
func (r *Repository) GetOrderByDraft(ctx context.Context, id string) (business.Order, error) {
	var value business.Order
	err := r.db.Collection("orders").FindOne(ctx, bson.M{"draft_id": id}).Decode(&value)
	return value, err
}
func (r *Repository) InsertOrder(ctx context.Context, value business.Order) error {
	_, err := r.db.Collection("orders").InsertOne(ctx, value)
	return err
}

// Seed 幂等写入固定 Demo 用户、陪玩和历史订单。
func (r *Repository) Seed(ctx context.Context, profiles []business.UserProfile, companions []business.Companion, orders []business.Order) error {
	for _, value := range profiles {
		if _, err := r.db.Collection("user_profiles").ReplaceOne(ctx, bson.M{"_id": value.UserID}, value, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
	}
	for _, value := range companions {
		if _, err := r.db.Collection("companions").ReplaceOne(ctx, bson.M{"_id": value.CompanionID}, value, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
	}
	for _, value := range orders {
		if _, err := r.db.Collection("orders").ReplaceOne(ctx, bson.M{"_id": value.OrderID}, value, options.Replace().SetUpsert(true)); err != nil {
			return err
		}
	}
	return r.EnsureIndexes(ctx)
}

// Reset 删除且只删除本 MVP 的 MongoDB 数据库。
func (r *Repository) Reset(ctx context.Context) error { return r.db.Drop(ctx) }
