package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"aws-cursor-router/internal/store"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

func (a *App) reloadAWSConfig(ctx context.Context) error {
	persistedCfg, exists, err := a.store.GetAWSConfig(ctx)
	if err != nil {
		return err
	}

	runtimeCfg := persistedCfg
	if !exists {
		runtimeCfg = store.AWSRuntimeConfig{
			Region:          a.cfg.AWSRegion,
			AccessKeyID:     a.cfg.AWSAccessKeyID,
			SecretAccessKey: a.cfg.AWSSecretAccessKey,
			SessionToken:    a.cfg.AWSSessionToken,
			DefaultModelID:  a.cfg.DefaultModelID,
		}
	}
	runtimeCfg.DefaultModelID = pickDefaultModelID(a.cfg.DefaultModelID, runtimeCfg.DefaultModelID)

	if strings.TrimSpace(runtimeCfg.Region) == "" {
		a.proxy.ReplaceClient(nil)
		a.proxy.SetDefaultModelID(runtimeCfg.DefaultModelID)
		a.setAWSRuntimeState(runtimeCfg, nil, nil)
		return nil
	}

	runtimeClient, controlClient, err := buildBedrockClients(ctx, runtimeCfg)
	if err != nil {
		return fmt.Errorf("initialize bedrock clients: %w", err)
	}

	a.proxy.ReplaceClient(runtimeClient)
	a.proxy.SetDefaultModelID(runtimeCfg.DefaultModelID)

	availableModels, err := fetchAvailableModels(ctx, controlClient, runtimeCfg.Region)
	if err != nil {
		a.logger.Printf("warning: failed to fetch available bedrock models: %v", err)
		availableModels = nil
	} else if len(availableModels) > 0 {
		if err := a.store.SeedEnabledModelsIfEmpty(ctx, availableModels); err != nil {
			return err
		}
	}

	a.setAWSRuntimeState(runtimeCfg, controlClient, availableModels)
	return nil
}

func (a *App) refreshAvailableModels(ctx context.Context) ([]string, error) {
	controlClient := a.getControlClient()
	if controlClient == nil {
		return nil, errors.New("bedrock control client is not configured")
	}

	awsCfg := a.getAWSConfig()
	region := strings.TrimSpace(awsCfg.Region)
	if region == "" {
		region = strings.TrimSpace(a.cfg.AWSRegion)
	}

	availableModels, err := fetchAvailableModels(ctx, controlClient, region)
	if err != nil {
		return nil, err
	}
	a.setAvailableModels(availableModels)

	if len(availableModels) > 0 {
		if err := a.store.SeedEnabledModelsIfEmpty(ctx, availableModels); err != nil {
			return nil, err
		}
	}

	return availableModels, nil
}

func (a *App) reloadEnabledModels(ctx context.Context) error {
	enabledModelIDs, err := a.store.ListEnabledModels(ctx)
	if err != nil {
		return err
	}

	if len(enabledModelIDs) == 0 {
		availableModels := a.listAvailableModels()
		if len(availableModels) > 0 {
			if err := a.store.SeedEnabledModelsIfEmpty(ctx, availableModels); err != nil {
				return err
			}
			enabledModelIDs, err = a.store.ListEnabledModels(ctx)
			if err != nil {
				return err
			}
		}
	}

	a.modelState.Replace(enabledModelIDs)
	return nil
}

func buildBedrockClients(ctx context.Context, runtimeCfg store.AWSRuntimeConfig) (*bedrockruntime.Client, *bedrock.Client, error) {
	runtimeCfg.Region = strings.TrimSpace(runtimeCfg.Region)
	runtimeCfg.AccessKeyID = strings.TrimSpace(runtimeCfg.AccessKeyID)
	runtimeCfg.SecretAccessKey = strings.TrimSpace(runtimeCfg.SecretAccessKey)
	runtimeCfg.SessionToken = strings.TrimSpace(runtimeCfg.SessionToken)

	if runtimeCfg.Region == "" {
		return nil, nil, errors.New("region is required")
	}

	loadOptions := []func(*awscfg.LoadOptions) error{awscfg.WithRegion(runtimeCfg.Region)}
	if runtimeCfg.AccessKeyID != "" || runtimeCfg.SecretAccessKey != "" {
		if runtimeCfg.AccessKeyID == "" || runtimeCfg.SecretAccessKey == "" {
			return nil, nil, errors.New("access_key_id and secret_access_key must be set together")
		}
		provider := credentials.NewStaticCredentialsProvider(
			runtimeCfg.AccessKeyID,
			runtimeCfg.SecretAccessKey,
			runtimeCfg.SessionToken,
		)
		loadOptions = append(loadOptions, awscfg.WithCredentialsProvider(provider))
	}

	sdkCfg, err := awscfg.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, nil, err
	}

	return bedrockruntime.NewFromConfig(sdkCfg), bedrock.NewFromConfig(sdkCfg), nil
}

func fetchAvailableModels(ctx context.Context, client foundationModelLister, region string) ([]string, error) {
	output, err := client.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
		ByOutputModality: bedrocktypes.ModelModalityText,
	})
	if err != nil {
		return nil, err
	}

	modelIDs := make([]string, 0, len(output.ModelSummaries))
	for _, summary := range output.ModelSummaries {
		modelID := strings.TrimSpace(awssdk.ToString(summary.ModelId))
		if modelID == "" {
			continue
		}
		if len(summary.InputModalities) > 0 && !containsModality(summary.InputModalities, bedrocktypes.ModelModalityText) {
			continue
		}
		if len(summary.OutputModalities) > 0 && !containsModality(summary.OutputModalities, bedrocktypes.ModelModalityText) {
			continue
		}
		modelIDs = append(modelIDs, modelID)
	}

	modelIDs = normalizeModelIDs(modelIDs)
	if !isNorthAmericaRegion(region) {
		return modelIDs, nil
	}

	withUSPrefix := make([]string, 0, len(modelIDs))
	for _, modelID := range modelIDs {
		if strings.HasPrefix(modelID, "us.") {
			withUSPrefix = append(withUSPrefix, modelID)
			continue
		}
		withUSPrefix = append(withUSPrefix, "us."+modelID)
	}
	return normalizeModelIDs(withUSPrefix), nil
}

func containsModality(modalities []bedrocktypes.ModelModality, target bedrocktypes.ModelModality) bool {
	for _, modality := range modalities {
		if modality == target {
			return true
		}
	}
	return false
}

func isNorthAmericaRegion(region string) bool {
	region = strings.ToLower(strings.TrimSpace(region))
	return strings.HasPrefix(region, "us-") ||
		strings.HasPrefix(region, "ca-") ||
		strings.HasPrefix(region, "mx-")
}
