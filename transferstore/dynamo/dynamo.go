package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"mq-transfer-go/transferstore"
)

// DynamoAPI describes the dynamodb client methods used by the store.
type DynamoAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
}

// DynamoStore implements TransferStore backed by DynamoDB

type DynamoStore struct {
	client DynamoAPI
	table  string
}

func NewDynamoStore(client DynamoAPI, table string) *DynamoStore {
	return &DynamoStore{client: client, table: table}
}

func (d *DynamoStore) Create(req transferstore.TransferRequest) error {
	item, err := attributevalue.MarshalMap(req)
	if err != nil {
		return err
	}
	_, err = d.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: &d.table,
		Item:      item,
	})
	return err
}

func (d *DynamoStore) GetByID(id string) (transferstore.TransferRequest, error) {
	out, err := d.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: &d.table,
		Key: map[string]types.AttributeValue{
			"RequestID": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return transferstore.TransferRequest{}, err
	}
	if out.Item == nil {
		return transferstore.TransferRequest{}, errors.New("not found")
	}
	var req transferstore.TransferRequest
	if err := attributevalue.UnmarshalMap(out.Item, &req); err != nil {
		return transferstore.TransferRequest{}, err
	}
	return req, nil
}

func (d *DynamoStore) UpdateStatus(id, status string, endTime *time.Time, errorMsg *string) error {
	expr := "SET #S = :s"
	attrs := map[string]types.AttributeValue{
		":s": &types.AttributeValueMemberS{Value: status},
	}
	names := map[string]string{"#S": "Status"}
	if endTime != nil {
		expr += ", EndTime = :e"
		attrs[":e"] = &types.AttributeValueMemberS{Value: endTime.Format(time.RFC3339)}
	}
	if errorMsg != nil {
		expr += ", #E = :err"
		attrs[":err"] = &types.AttributeValueMemberS{Value: *errorMsg}
		names["#E"] = "Error"
	}
	_, err := d.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName:                 &d.table,
		Key:                       map[string]types.AttributeValue{"RequestID": &types.AttributeValueMemberS{Value: id}},
		UpdateExpression:          &expr,
		ExpressionAttributeValues: attrs,
		ExpressionAttributeNames:  names,
	})
	return err
}

func (d *DynamoStore) UpdateProgress(id string, messagesTransferred int, bytesTransferred int) error {
	expr := "SET MessagesTransferred = :m, BytesTransferred = :b"
	_, err := d.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName:        &d.table,
		Key:              map[string]types.AttributeValue{"RequestID": &types.AttributeValueMemberS{Value: id}},
		UpdateExpression: &expr,
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":m": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", messagesTransferred)},
			":b": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", bytesTransferred)},
		},
	})
	return err
}

func (d *DynamoStore) List() ([]transferstore.TransferRequest, error) {
	out, err := d.client.Scan(context.Background(), &dynamodb.ScanInput{TableName: &d.table})
	if err != nil {
		return nil, err
	}
	var reqs []transferstore.TransferRequest
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &reqs); err != nil {
		return nil, err
	}
	return reqs, nil
}
