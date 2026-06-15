package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/kyou-id/yukari/internal/audit"
	"github.com/kyou-id/yukari/internal/config"
	"github.com/kyou-id/yukari/internal/reader"
	"github.com/kyou-id/yukari/internal/repository"
	"github.com/kyou-id/yukari/internal/sqlfiles"
)

func buildStore(cfg config.Config, now time.Time) (reader.Store, error) {
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("DATABASE_DSN is empty; using stub repository")
		return repository.NewStubStore(now), nil
	}
	return repository.OpenMySQLStore(cfg.DatabaseDSN, sqlfiles.NewLoader(cfg.SQLDir))
}

func buildAuditLogger(cfg config.Config) (*audit.Logger, error) {
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("OLD_DATABASE_* is empty; running without email delivery audit logs")
		return nil, nil
	}
	return audit.Open(cfg.DatabaseDSN)
}

func buildVoucherCreator(cfg config.Config) (*repository.MySQLVoucherCreator, error) {
	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.VoucherConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("voucher config %s not found; enqueuing jobs without creating vouchers", cfg.VoucherConfigPath)
			return nil, nil
		}
		return nil, err
	}
	if !voucherCfg.PricingVoucherID.Valid && strings.TrimSpace(voucherCfg.PricingVoucherCode) == "" {
		log.Printf("voucher config %s has no pricing_voucher_id or pricing_voucher_code; enqueuing jobs without creating vouchers", cfg.VoucherConfigPath)
		return nil, nil
	}
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("OLD_DATABASE_* is empty; enqueuing jobs without creating vouchers")
		return nil, nil
	}
	return repository.OpenMySQLVoucherCreator(cfg.DatabaseDSN, voucherCfg, cfg.VoucherCodeSecret)
}

func buildAnniversaryVoucherCreator(cfg config.Config) (*repository.MySQLVoucherCreator, error) {
	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.AnniversaryVoucherConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("anniversary voucher config %s not found; enqueuing anniversary jobs without creating vouchers", cfg.AnniversaryVoucherConfigPath)
			return nil, nil
		}
		return nil, err
	}
	if !voucherCfg.PricingVoucherID.Valid && strings.TrimSpace(voucherCfg.PricingVoucherCode) == "" {
		log.Printf("anniversary voucher config %s has no pricing_voucher_id or pricing_voucher_code; enqueuing without creating vouchers", cfg.AnniversaryVoucherConfigPath)
		return nil, nil
	}
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("OLD_DATABASE_* is empty; enqueuing anniversary jobs without creating vouchers")
		return nil, nil
	}
	return repository.OpenMySQLVoucherCreator(cfg.DatabaseDSN, voucherCfg, cfg.VoucherCodeSecret)
}
