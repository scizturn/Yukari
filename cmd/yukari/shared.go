package main

import (
	"fmt"
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

func buildWinbackVoucherCreator(cfg config.Config) (*repository.MySQLVoucherCreator, error) {
	voucherCfg, err := repository.LoadBirthdayVoucherConfig(cfg.WinbackVoucherConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("winback voucher config %s not found; enqueuing winback jobs without creating vouchers", cfg.WinbackVoucherConfigPath)
			return nil, nil
		}
		return nil, err
	}
	if !voucherCfg.PricingVoucherID.Valid && strings.TrimSpace(voucherCfg.PricingVoucherCode) == "" {
		log.Printf("winback voucher config %s has no pricing_voucher_id or pricing_voucher_code; enqueuing without creating vouchers", cfg.WinbackVoucherConfigPath)
		return nil, nil
	}
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		log.Print("OLD_DATABASE_* is empty; enqueuing winback jobs without creating vouchers")
		return nil, nil
	}
	return repository.OpenMySQLVoucherCreator(cfg.DatabaseDSN, voucherCfg, cfg.VoucherCodeSecret)
}

// wishlistBackInTierConfigs loads both GP tiers. Unlike the other campaigns this
// returns nil (enqueue without vouchers) only when BOTH configs are unusable —
// a half-configured pair would silently downgrade every user to one tier.
func wishlistBackInTierConfigs(cfg config.Config) (map[int]repository.BirthdayVoucherConfig, error) {
	paths := map[int]string{
		8: cfg.WishlistBackInVoucherConfigPath,
		6: cfg.WishlistBackInLowVoucherConfigPath,
	}
	configs := make(map[int]repository.BirthdayVoucherConfig, len(paths))
	for percent, path := range paths {
		voucherCfg, err := repository.LoadBirthdayVoucherConfig(path)
		if err != nil {
			if os.IsNotExist(err) {
				log.Printf("wishlist back in %d%% voucher config %s not found", percent, path)
				continue
			}
			return nil, err
		}
		if !voucherCfg.PricingVoucherID.Valid && strings.TrimSpace(voucherCfg.PricingVoucherCode) == "" {
			log.Printf("wishlist back in %d%% voucher config %s has no pricing voucher id or code", percent, path)
			continue
		}
		configs[percent] = voucherCfg
	}
	return configs, nil
}

func buildWishlistBackInVoucherCreator(cfg config.Config) (*repository.WishlistBackInCreator, error) {
	configs, err := wishlistBackInTierConfigs(cfg)
	if err != nil {
		return nil, err
	}
	if len(configs) == 0 {
		log.Print("no usable wishlist back in voucher config; enqueuing without vouchers")
		return nil, nil
	}
	if len(configs) < 2 {
		// Minting only one tier would hand a GP-30 item an 8% voucher it cannot
		// use, or bill a GP-40 item at 6%. Neither is a silent-degradation case.
		return nil, fmt.Errorf("wishlist back in needs both voucher tiers configured, got %d", len(configs))
	}
	if strings.TrimSpace(cfg.DatabaseDSN) == "" {
		return nil, nil
	}
	return repository.OpenWishlistBackInCreator(cfg.DatabaseDSN, configs, cfg.VoucherCodeSecret)
}
