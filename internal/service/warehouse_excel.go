package service

import (
	"fmt"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"

	"github.com/xuri/excelize/v2"
)

func buildWarehouseExcel(records []model.WarehouseBillingRecord, accountMap map[uint64]repository.AccountBasic) ([]byte, error) {
	f := excelize.NewFile()
	sheet := "云仓流水"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"发生时间", "账号", "姓名", "云仓流水单号", "平台", "店铺名",
		"关联订单号", "业务类型", "类型", "运费", "打包费", "总金额", "件数", "状态", "交易前余额", "交易后余额",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"DDEBF7"}, Pattern: 1},
	})
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%s1", string(rune('A'+len(headers)-1))), style)

	for i, rec := range records {
		row := i + 2
		info := accountMap[rec.AccountID]
		typeLabel := "扣款"
		if rec.Type == "recharge" {
			typeLabel = "充值"
		}
		values := []interface{}{
			rec.CreatedAt.Format("2006-01-02 15:04:05"),
			info.Username,
			info.RealName,
			rec.FlowNo,
			rec.Platform,
			rec.ShopName,
			rec.TradeNo,
			rec.BusinessType,
			typeLabel,
			rec.ShippingFee,
			rec.PackingFee,
			rec.TotalAmount,
			rec.ItemCount,
			rec.Status,
			rec.BalanceBefore,
			rec.BalanceAfter,
		}
		for j, v := range values {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	for i := range headers {
		col := string(rune('A' + i))
		f.SetColWidth(sheet, col, col, 18)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildMyWarehouseExcel(records []model.WarehouseBillingRecord) ([]byte, error) {
	f := excelize.NewFile()
	sheet := "云仓流水"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"发生时间", "云仓流水单号", "平台", "店铺名", "关联订单号",
		"业务类型", "类型", "运费", "打包费", "总金额", "件数", "状态", "交易后余额",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"DDEBF7"}, Pattern: 1},
	})
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%s1", string(rune('A'+len(headers)-1))), style)

	for i, rec := range records {
		row := i + 2
		typeLabel := "扣款"
		if rec.Type == "recharge" {
			typeLabel = "充值"
		}
		values := []interface{}{
			rec.CreatedAt.Format("2006-01-02 15:04:05"),
			rec.FlowNo,
			rec.Platform,
			rec.ShopName,
			rec.TradeNo,
			rec.BusinessType,
			typeLabel,
			rec.ShippingFee,
			rec.PackingFee,
			rec.TotalAmount,
			rec.ItemCount,
			rec.Status,
			rec.BalanceAfter,
		}
		for j, v := range values {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	for i := range headers {
		col := string(rune('A' + i))
		f.SetColWidth(sheet, col, col, 18)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
