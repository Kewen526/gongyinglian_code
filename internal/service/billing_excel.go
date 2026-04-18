package service

import (
	"fmt"
	"supply-chain/internal/model"
	"supply-chain/internal/repository"

	"github.com/xuri/excelize/v2"
)

// typeLabel maps billing record type to Chinese display name.
func typeLabel(t string) string {
	switch t {
	case "recharge":
		return "充值"
	case "deduct":
		return "订单扣款"
	case "refund":
		return "售后退款"
	default:
		return t
	}
}

// statusLabel maps billing record status to Chinese display name.
func statusLabel(s string) string {
	switch s {
	case "success":
		return "成功"
	case "insufficient":
		return "余额不足"
	case "error":
		return "货号错误"
	default:
		return s
	}
}

// buildBillingExcel creates an Excel workbook from billing records and returns the raw bytes.
func buildBillingExcel(records []model.BillingRecord, accountMap map[uint64]repository.AccountBasic) ([]byte, error) {
	f := excelize.NewFile()
	sheet := "资金流水"
	f.SetSheetName("Sheet1", sheet)

	// Header row
	headers := []string{
		"发生时间", "账号", "姓名", "店铺", "订单号", "流水号",
		"类型", "原价", "折扣率", "优惠金额", "实际金额", "状态", "交易前余额", "交易后余额",
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	// Style: bold header
	style, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"DDEBF7"}, Pattern: 1},
	})
	f.SetCellStyle(sheet, "A1", fmt.Sprintf("%s1", string(rune('A'+len(headers)-1))), style)

	// Data rows
	for i, rec := range records {
		row := i + 2
		info := accountMap[rec.AccountID]
		values := []interface{}{
			rec.CreatedAt.Format("2006-01-02 15:04:05"),
			info.Username,
			info.RealName,
			rec.ShopName,
			rec.TradeNo,
			rec.FlowNo,
			typeLabel(rec.Type),
			rec.OriginalAmount,
			rec.DiscountRate,
			rec.DiscountAmount,
			rec.ActualAmount,
			statusLabel(rec.Status),
			rec.BalanceBefore,
			rec.BalanceAfter,
		}
		for j, v := range values {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	// Auto-width for key columns
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

// buildMyBillingExcel creates an Excel workbook for a single employee's billing records.
func buildMyBillingExcel(records []model.BillingRecord) ([]byte, error) {
	f := excelize.NewFile()
	sheet := "我的账单"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{
		"发生时间", "客户资金流水号", "平台", "店铺名", "订单号",
		"类型", "状态", "金额", "交易后余额",
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
		values := []interface{}{
			rec.CreatedAt.Format("2006-01-02 15:04:05"),
			rec.FlowNo,
			rec.Platform,
			rec.ShopName,
			rec.TradeNo,
			typeLabel(rec.Type),
			statusLabel(rec.Status),
			rec.ActualAmount,
			rec.BalanceAfter,
		}
		for j, v := range values {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	for i := range headers {
		col := string(rune('A' + i))
		f.SetColWidth(sheet, col, col, 20)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
