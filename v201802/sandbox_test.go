package v201802

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

const (
	TARGETING_IDEA_LIMIT = 100
)

func getTestConfig() AuthConfig {

	creds := Credentials{
		Config: OAuthConfigArgs{
			ClientID:     os.Getenv("ADWORDS_CLIENT_ID"),
			ClientSecret: os.Getenv("ADWORDS_CLIENT_SECRET"),
		},
		Token: OAuthTokenArgs{
			AccessToken:  os.Getenv("ADWORDS_ACCESS_TOKEN"),
			RefreshToken: os.Getenv("ADWORDS_REFRESH_TOKEN"),
		},
		Auth: Auth{
			CustomerId:     os.Getenv("ADWORDS_TEST_ACCOUNT"),
			DeveloperToken: os.Getenv("ADWORDS_DEVELOPER_TOKEN"),
			PartialFailure: true,
		},
	}

	authconf, _ := NewCredentialsFromParams(creds)
	return authconf
}

func createUniqueId() int64 {
	return time.Now().UnixNano() * -1
}

func createBatchTextAdOperation(adgroupId int64) (operations AdGroupAdOperations) {
	fmt.Printf("using adgroup id: %d\n", adgroupId)
	return AdGroupAdOperations{
		"ADD": {
			BatchExpandedTextAd{
				AdGroupId:     adgroupId,
				FinalUrls:     []string{"https://getsidecar.com"},
				HeadlinePart1: "Buy Now | Sidecar",
				HeadlinePart2: "Buy something now",
				Description:   "Great deal",
				Path1:         "Data",
				Path2:         "Apps",
				Status:        "PAUSED",
				Type:          "ExpandedTextAd",
			},
		},
	}
}

func createBatchOperations(num int) (operations []interface{}) {
	campaignId := createUniqueId()
	campaignOp := CampaignOperations{
		"ADD": {
			Campaign{
				Id:                     campaignId,
				Name:                   fmt.Sprintf("dave's batch test campaign%d", num),
				Status:                 "PAUSED",
				StartDate:              time.Now().Format("20060102"),
				BudgetId:               1329921755,
				AdvertisingChannelType: "SEARCH",
				BiddingStrategyConfiguration: &BiddingStrategyConfiguration{
					StrategyType: "MANUAL_CPC",
				},
			},
		},
	}

	operations = append(operations, campaignOp)

	adgroupId := createUniqueId()

	adgroupOp := AdGroupOperations{
		"ADD": {
			AdGroup{
				Id:         adgroupId,
				Name:       fmt.Sprintf("dave's batch test adgroup%d", num),
				Status:     "ENABLED",
				CampaignId: campaignId,
			},
		},
	}

	operations = append(operations, adgroupOp)

	operations = append(operations, createBatchTextAdOperation(adgroupId))

	keywordOps := AdGroupCriterionOperations{
		"ADD": {
			BiddableAdGroupCriterion{
				AdGroupId:  adgroupId,
				Criterion:  KeywordCriterion{Text: fmt.Sprintf("+positive +keyword%d", num), MatchType: "BROAD"},
				UserStatus: "ENABLED",
				BiddingStrategyConfiguration: &BiddingStrategyConfiguration{
					Bids: []Bid{
						Bid{
							Type:   "CpcBid",
							Amount: 1000000,
						},
					},
				},
			},
			NegativeAdGroupCriterion{
				AdGroupId: adgroupId,
				Criterion: KeywordCriterion{Text: fmt.Sprintf("+negative +keyword%d", num), MatchType: "BROAD"},
			},
		},
	}

	operations = append(operations, keywordOps)

	return operations
}

func TestSandboxBatchJobTesting(t *testing.T) {
	config := getTestConfig()
	srv := NewBatchJobService(&config.Auth)

	// Create batch job
	resp, err := srv.Mutate(
		BatchJobOperations{
			BatchJobOperations: []BatchJobOperation{
				BatchJobOperation{
					Operator: "ADD",
					Operand:  BatchJob{},
				},
			},
		},
	)

	if err != nil {
		t.Errorf("didn't expect an error: %v", err)
	}
	job := resp[0]

	// Create operations
	var operations []interface{}
	TOTAL_SIZE := 1
	for i := 0; i < TOTAL_SIZE; i++ {
		operations = append(operations, createBatchOperations(i)...)
	}

	// Upload the operations
	fmt.Printf("uploading %d operations...", len(operations))
	helperSrv := NewBatchJobHelper(&config.Auth)
	err = helperSrv.UploadBatchJobOperations(operations, *job.UploadUrl)
	if err != nil {
		t.Errorf("didn't expect an error: %v", err)
	}
	fmt.Println("done!")
	start := time.Now()

	SLEEP := 15

	for {
		resp, err := srv.Get(
			Selector{
				Fields: []string{
					"Id",
					"Status",
					"DownloadUrl",
					"ProcessingErrors",
					"ProgressStats",
				},
				Predicates: []Predicate{
					{"Id", "EQUALS", []string{strconv.FormatInt(job.Id, 10)}},
				},
			},
		)

		if err != nil {
			t.Errorf("didn't expect there to be an error: %v", err)
		}

		if resp.BatchJobs[0].Status != "DONE" {
			fmt.Printf("got a status of %s, sleeping and retrying\n", resp.BatchJobs[0].Status)
			time.Sleep(time.Duration(SLEEP) * time.Second)
			continue
		}

		if resp.TotalNumEntries != 1 {
			t.Errorf("expected there to be one entry, but got %d", resp.TotalNumEntries)
		}

		fmt.Println("operations have completed processing")
		end := int(time.Now().Sub(start).Seconds())
		fmt.Printf("took %d seconds\n", end)

		results, err := helperSrv.DownloadBatchJob(*resp.BatchJobs[0].DownloadUrl)
		if err != nil {
			t.Errorf("didn't expect an error: %v", err)
		}

		for _, result := range results {
			if len(result.ErrorList) > 0 {
				for _, e := range result.ErrorList {
					t.Errorf("error returned for entity %s: %s", e.Errors.Trigger, e.Errors.ErrorString)
				}
			}
		}

		// delete the new campaign
		firstResult := results[0].Result
		newCampaign := firstResult.(Campaign)
		newCampaign.Status = "REMOVED"
		newCampaign.AdServingOptimizationStatus = ""

		campaignSrv := NewCampaignService(&config.Auth)
		_, err = campaignSrv.Mutate(CampaignOperations{
			"SET": {
				newCampaign,
			},
		})
		if err != nil {
			t.Errorf("didn't expect an error when deleting the new campaign: %v", err)
		}

		return
	}
}

// NOTE: When running this on a non-production account you won't get real results
// just stuff like "keyword XXXXXXXX" or "red herring XXXXXXXX"
// https://groups.google.com/forum/#!msg/adwords-api/PVVYUY421yA/_yZMgEg5PiUJ
func TestSandboxTargetingIdeaKeywords(t *testing.T) {
	config := getTestConfig()
	srv := NewTargetingIdeaService(&config.Auth)

	selector := TargetingIdeaSelector{
		SearchParameters: []SearchParameter{
			RelatedToQuerySearchParameter{
				Queries: []string{"flowers"},
			},
			NetworkSearchParameter{
				NetworkSetting: NetworkSetting{
					TargetGoogleSearch:         true,
					TargetSearchNetwork:        true,
					TargetContentNetwork:       false,
					TargetPartnerSearchNetwork: false,
				},
			},
		},
		IdeaType:                "KEYWORD",
		RequestedAttributeTypes: []string{"KEYWORD_TEXT"},
		RequestType:             "IDEAS",
		Paging:                  Paging{0, int64(TARGETING_IDEA_LIMIT)},
	}
	ideas, count, err := srv.Get(selector)
	if err != nil {
		t.Fatalf("didn't expect an error: %v", err)
	}

	if len(ideas) != TARGETING_IDEA_LIMIT {
		t.Fatalf("expected %d ideas to be returned", TARGETING_IDEA_LIMIT)
	}

	if count < int64(TARGETING_IDEA_LIMIT) {
		t.Fatalf("expected the total idea count to be at least the paging limit of %d, but got %d", TARGETING_IDEA_LIMIT, count)
	}

	fmt.Println("sample of keywords returned:")
	for _, idea := range ideas[0:5] {
		fmt.Println(idea.TargetingIdea[0].Value)
	}
}

func TestSandboxTargetingIdeaURLs(t *testing.T) {
	config := getTestConfig()
	srv := NewTargetingIdeaService(&config.Auth)

	selector := TargetingIdeaSelector{
		SearchParameters: []SearchParameter{
			RelatedToUrlSearchParameter{
				Urls:           []string{"https://getsidecar.com/"},
				IncludeSubUrls: false,
			},
			NetworkSearchParameter{
				NetworkSetting: NetworkSetting{
					TargetGoogleSearch:         true,
					TargetSearchNetwork:        true,
					TargetContentNetwork:       false,
					TargetPartnerSearchNetwork: false,
				},
			},
		},
		IdeaType:                "KEYWORD",
		RequestedAttributeTypes: []string{"KEYWORD_TEXT"},
		RequestType:             "IDEAS",
		Paging:                  Paging{0, int64(TARGETING_IDEA_LIMIT)},
	}
	ideas, count, err := srv.Get(selector)
	if err != nil {
		t.Fatalf("didn't expect an error: %v", err)
	}

	if len(ideas) != TARGETING_IDEA_LIMIT {
		t.Fatalf("expected %d ideas to be returned", TARGETING_IDEA_LIMIT)
	}

	if count < int64(TARGETING_IDEA_LIMIT) {
		t.Fatalf("expected the total idea count to be at least the paging limit of %d, but got %d", TARGETING_IDEA_LIMIT, count)
	}

	fmt.Println("sample of keywords returned:")
	for _, idea := range ideas[0:5] {
		fmt.Println(idea.TargetingIdea[0].Value)
	}
}

func TestSandboxTrafficEstimator(t *testing.T) {
	config := getTestConfig()
	estimator := NewTrafficEstimatorService(&config.Auth)

	isEstimateEmpty := func(estimate KeywordEstimate) bool {
		isEmpty := func(sEstimate StatsEstimate) bool {
			if sEstimate.AverageCpc == 0 {
				return true
			}

			if sEstimate.AveragePosition == 0.0 {
				return true
			}

			if sEstimate.ClickThroughRate == 0.0 {
				return true
			}

			if sEstimate.ClicksPerDay == 0.0 {
				return true
			}

			if sEstimate.ImpressionsPerDay == 0.0 {
				return true
			}

			if sEstimate.TotalCost == 0 {
				return true
			}

			return false
		}

		empty := isEmpty(estimate.Min)
		if empty {
			return empty
		}

		empty = isEmpty(estimate.Max)
		return empty
	}

	selector := TrafficEstimatorSelector{
		CampaignEstimateRequests: []CampaignEstimateRequest{
			CampaignEstimateRequest{
				AdGroupEstimateRequests: []AdGroupEstimateRequest{
					AdGroupEstimateRequest{
						KeywordEstimateRequests: []KeywordEstimateRequest{
							KeywordEstimateRequest{
								KeywordCriterion{
									Text:      "peony artificial flowers",
									MatchType: "BROAD",
								},
							},
							KeywordEstimateRequest{
								KeywordCriterion{
									Text:      "artificial gerbera flowers",
									MatchType: "BROAD",
								},
							},
						},
						MaxCpc: 1000000,
					},
				},
				DailyBudget: 100000,
			},
		}}

	resp, err := estimator.Get(selector)

	if err != nil {
		t.Fatalf("didn't expect an error: %v", err)
	}

	if len(resp[0].AdGroupEstimates[0].KeywordEstimates) != 2 {
		t.Fatal("expected estimations for each keyword")
	}

	for _, k := range resp[0].AdGroupEstimates[0].KeywordEstimates {
		empty := isEstimateEmpty(k)
		if empty {
			t.Fatalf("keyword estimate has null value(s): %+v\n", k)
		}
	}
}

func TestSandboxCreateSharedSet(t *testing.T) {
	config := getTestConfig()

	sets, err := NewSharedSetService(&config.Auth).Mutate([]SharedSetOperation{
		{Operator: "ADD", Operand: SharedSet{Name: "created-shared-set-1", Type: "NEGATIVE_KEYWORDS"}},
		{Operator: "ADD", Operand: SharedSet{Name: "created-shared-set-2", Type: "NEGATIVE_KEYWORDS"}},
	})

	if err != nil {
		t.Fatal(err)
	}

	ops := make([]SharedSetOperation, len(sets))

	for i := range sets {
		ops[i].Operand = sets[i]
		ops[i].Operator = "REMOVE"
	}

	_, err = NewSharedSetService(&config.Auth).Mutate(ops)
	if err != nil {
		t.Error(err)
	}
}

func TestOPPBreakout(t *testing.T) {
	config := getTestConfig()

	campaigns, _, err := NewCampaignService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name", "CampaignId"},
	})

	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(campaigns)
	campaignId := campaigns[0].Id

	/*
		adgroups, err := NewAdGroupService(&config.Auth).Mutate(AdGroupOperations{
			"ADD": []AdGroup{
				AdGroup{
					Name:       "opp-breakout-test",
					Status:     "PAUSED",
					CampaignId: campaignId,
				},
			}})
	*/

	adgroups, _, err := NewAdGroupService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
		Predicates: []Predicate{
			Predicate{
				Field:    "CampaignId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(campaignId, 10)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	adgroup, err := func() (*AdGroup, error) {
		for _, a := range adgroups {
			if a.Name == "opp-breakout-test" {
				return &a, nil
			}
		}
		return nil, fmt.Errorf("missing test adgroup\n")
	}()

	crits, _, err := NewAdGroupCriterionService(&config.Auth).Get(Selector{
		Fields: []string{"AdGroupId", "BidModifier", "CriterionUse", "ParentCriterionId", "CriteriaType", "CaseValue", "Id", "BiddingStrategyType", "CpcBid", "BiddingStrategyId"},
		Predicates: []Predicate{
			Predicate{
				Field:    "AdGroupId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(adgroup.Id, 10)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, x := range crits {
		fmt.Printf("%#v\n", x)
	}

	var target BiddableAdGroupCriterion
	var rootId int64

	for i := 0; i < len(crits); i++ {
		crit, _ := crits[i].(BiddableAdGroupCriterion)
		part := crit.Criterion.(ProductPartition)
		fmt.Printf("%#v\n", part)

		if part.ParentCriterionId == 0 {
			rootId = part.Id
		}

		if part.Dimension.Value == "" && part.Dimension.Type == "ProductBrand" {
			target = crit
		}
	}

	fmt.Println("target ---------------------->")
	fmt.Println(target)
	bsc := &BiddingStrategyConfiguration{
		StrategyType: "NONE",
		Bids: []Bid{
			Bid{Type: "CpcBid", Amount: 60000},
		},
	}

	newopp := BiddableAdGroupCriterion{
		AdGroupId: adgroup.Id,
		Criterion: ProductPartition{
			Id:                -501,
			CriteriaType:      "",
			PartitionType:     "SUBDIVISION",
			ParentCriterionId: rootId,
			Dimension: ProductDimension{
				Type:  "ProductBrand",
				Value: "",
			},
		},
	}

	child := BiddableAdGroupCriterion{
		AdGroupId: adgroup.Id,
		Criterion: ProductPartition{
			CriteriaType:      "PRODUCT_PARTITION",
			PartitionType:     "UNIT",
			ParentCriterionId: -501,
			Dimension: ProductDimension{
				Type:  "ProductOfferId",
				Value: "ASDF0001",
			},
		},
		BiddingStrategyConfiguration: bsc,
	}

	oppopp := BiddableAdGroupCriterion{
		AdGroupId: adgroup.Id,
		Criterion: ProductPartition{
			CriteriaType:      "PRODUCT_PARTITION",
			PartitionType:     "UNIT",
			ParentCriterionId: -501,
			Dimension: ProductDimension{
				Type:  "ProductOfferId",
				Value: "",
			},
		},
		BiddingStrategyConfiguration: bsc,
	}

	aops := []AdGroupCriterionOperation{
		{"REMOVE", target},
		{"ADD", newopp},
		{"ADD", oppopp},
		{"ADD", child},
	}

	config.Auth.ValidateOnly = true
	/*
		root := BiddableAdGroupCriterion{
			AdGroupId: adgroup.Id,
			Criterion: ProductPartition{
				Id:                -555,
				CriteriaType:      "",
				PartitionType:     "SUBDIVISION",
				ParentCriterionId: 0,
			},
		}

		part1 := BiddableAdGroupCriterion{
			AdGroupId: adgroup.Id,
			Criterion: ProductPartition{
				CriteriaType:      "PRODUCT_PARTITION",
				PartitionType:     "UNIT",
				ParentCriterionId: -555,
				Dimension: ProductDimension{
					Type:  "ProductBrand",
					Value: "int",
				},
			},
			BiddingStrategyConfiguration: bsc,
		}

		part := BiddableAdGroupCriterion{
			AdGroupId: adgroup.Id,
			Criterion: ProductPartition{
				CriteriaType:      "PRODUCT_PARTITION",
				PartitionType:     "UNIT",
				ParentCriterionId: -555,
				Dimension: ProductDimension{
					Type:  "ProductBrand",
					Value: "agi",
				},
			},
			BiddingStrategyConfiguration: bsc,
		}

		opp := BiddableAdGroupCriterion{
			AdGroupId: adgroup.Id,
			Criterion: ProductPartition{
				CriteriaType:      "PRODUCT_PARTITION",
				PartitionType:     "UNIT",
				ParentCriterionId: -555,
				Dimension: ProductDimension{
					Type:  "ProductBrand",
					Value: "",
				},
			},
			BiddingStrategyConfiguration: bsc,
		}

		aops := []AdGroupCriterionOperation{
			{"ADD", root},
			{"ADD", opp},
			{"ADD", part1},
			{"ADD", part},
		}
	*/

	res, err := NewAdGroupCriterionService(&config.Auth).MutateOperations(aops)

	fmt.Println(err, res)

}

func TestBreakOut(t *testing.T) {
	config := getTestConfig()

	campaigns, _, err := NewCampaignService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name", "CampaignId"},
	})

	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(campaigns)
	campaign := campaigns[0].Id

	adgroups, _, err := NewAdGroupService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
		Predicates: []Predicate{
			Predicate{
				Field:    "CampaignId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(campaign, 10)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	adgroup, err := func() (*AdGroup, error) {
		for _, a := range adgroups {
			if a.Name == "sidecar-test-adgroup" {
				return &a, nil
			}
		}
		return nil, fmt.Errorf("missing test adgroup\n")
	}()
	if err != nil {
		t.Fatal(err)
	}
	/*
		query := fmt.Sprintf("SELECT * WHERE AdGroupId = %d", adgroup.Id)

		crits, _, err := NewAdGroupCriterionService(&config.Auth).Query(query)
	*/
	crits, _, err := NewAdGroupCriterionService(&config.Auth).Get(Selector{
		Fields: []string{"AdGroupId", "BidModifier", "CriterionUse", "ParentCriterionId", "CriteriaType", "CaseValue", "Id", "BiddingStrategyType", "CpcBid", "BiddingStrategyId"},
		Predicates: []Predicate{
			Predicate{
				Field:    "AdGroupId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(adgroup.Id, 10)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var root BiddableAdGroupCriterion

	for i := 0; i < len(crits); i++ {
		crit, _ := crits[i].(BiddableAdGroupCriterion)
		part := crit.Criterion.(ProductPartition)
		fmt.Printf("%#v\n", part)

		if part.Dimension.Value == "aaa" || part.Dimension.Value == "bbb" {
			root = crit
		}
	}

	crit := root.Criterion.(ProductPartition)
	crit.PartitionType = "SUBDIVISION"
	root.Criterion = crit

	bsc := &BiddingStrategyConfiguration{
		StrategyType: "NONE",
		Bids: []Bid{
			Bid{Type: "CpcBid", Amount: 600000},
		},
	}

	cpc := &Cpc{
		Amount: &CpcAmount{
			MicroAmount: 600000,
		},
	}

	newroot := root
	newcrit := crit
	newcrit.Id = -100
	newcrit.Dimension.Value = "aaa"
	//newcrit.Cpc = cpc
	newroot.Criterion = newcrit
	newroot.BiddingStrategyConfiguration = nil
	//newroot.BiddingStrategyConfiguration.StrategyType = "NONE"

	//newroot.BiddingStrategyConfiguration = bsc

	newpart := BiddableAdGroupCriterion{
		AdGroupId: root.AdGroupId,
		Criterion: ProductPartition{
			CriteriaType:      "PRODUCT_PARTITION",
			PartitionType:     "UNIT",
			ParentCriterionId: newcrit.Id,
			Dimension: ProductDimension{
				Type:  "ProductCanonicalCondition",
				Value: "NEW",
			},
			Cpc: cpc,
		},
		BiddingStrategyConfiguration: bsc,
	}

	opp := BiddableAdGroupCriterion{
		AdGroupId: root.AdGroupId,
		Criterion: ProductPartition{
			CriteriaType:      "PRODUCT_PARTITION",
			PartitionType:     "UNIT",
			ParentCriterionId: newcrit.Id,
			Dimension: ProductDimension{
				Type:  "ProductCanonicalCondition",
				Value: "",
			},
		},
		BiddingStrategyConfiguration: bsc,
	}

	aops := []AdGroupCriterionOperation{
		{"REMOVE", root},
		{"ADD", newroot},
		{"ADD", opp},
		{"ADD", newpart},
	}

	res, err := NewAdGroupCriterionService(&config.Auth).MutateOperations(aops)

	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(res)
}

func TestSandboxCriteria(t *testing.T) {
	config := getTestConfig()

	campaigns, _, err := NewCampaignService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name", "CampaignId"},
	})

	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(campaigns)
	campaign := campaigns[0].Id

	adgroups, _, err := NewAdGroupService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
		Predicates: []Predicate{
			Predicate{
				Field:    "CampaignId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(campaign, 10)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	adgroup, err := func() (*AdGroup, error) {
		for _, a := range adgroups {
			if a.Name == "sidecar-test-adgroup" {
				return &a, nil
			}
		}
		return nil, fmt.Errorf("missing test adgroup\n")
	}()
	if err != nil {
		t.Fatal(err)
	}
	/*
		query := fmt.Sprintf("SELECT * WHERE AdGroupId = %d", adgroup.Id)

		crits, _, err := NewAdGroupCriterionService(&config.Auth).Query(query)
	*/
	crits, _, err := NewAdGroupCriterionService(&config.Auth).Get(Selector{
		Fields: []string{"AdGroupId", "BidModifier", "CriterionUse", "ParentCriterionId", "CriteriaType", "CaseValue", "Id", "BiddingStrategyType", "CpcBid", "BiddingStrategyId"},
		Predicates: []Predicate{
			Predicate{
				Field:    "AdGroupId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(adgroup.Id, 10)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	//rootCriterion

	root, rest, toremove := func() (ProductPartition, []BiddableAdGroupCriterion, *BiddableAdGroupCriterion) {
		var root ProductPartition
		var rest []BiddableAdGroupCriterion
		var toremove *BiddableAdGroupCriterion

		for i := 0; i < len(crits); i++ {
			crit, _ := crits[i].(BiddableAdGroupCriterion)
			part := crit.Criterion.(ProductPartition)

			if part.ParentCriterionId == 0 {
				root = part
			} else {
				crit.Criterion = part
				fmt.Printf("CRIT:  %#v\n%#v\n", crit, *crit.BiddingStrategyConfiguration)
				if part.Dimension.Value == "agi" {
					//	part.Dimension.TypeAttr = "ProductBrand"
					toremove = &crit
				} else {
					rest = append(rest, crit)
				}
			}
		}
		return root, rest, toremove
	}()

	fmt.Printf("ROOT:  %#v\n", root)

	/*
		removes := AdGroupCriterions{}
		for _, x := range rest {
			removes = append(removes, BiddableAdGroupCriterion{
				AdGroupId: x.AdGroupId,
				Criterion: ProductPartition{
					Id: x.Criterion.(ProductPartition).Id,
				},
			})
		}
	*/

	if toremove != nil {
		aops := []AdGroupCriterionOperation{
			{"REMOVE", *toremove},
		}

		res, err := NewAdGroupCriterionService(&config.Auth).MutateOperations(aops)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println(res)
	}

	toadd := rest[0]
	part := toadd.Criterion.(ProductPartition)
	part.Dimension.Value = "agi"
	part.Id = 0
	toadd.Criterion = part
	toadd.BiddingStrategyConfiguration.StrategyType = "NONE"
	//toadd.BiddingStrategyConfiguration = nil

	aops := []AdGroupCriterionOperation{
		{"ADD", toadd},
	}

	res, err := NewAdGroupCriterionService(&config.Auth).MutateOperations(aops)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(res)
}

func TestSandboxValidateOnly(t *testing.T) {
	config := getTestConfig()
	campaigns, n, err := NewCampaignService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
	})

	fmt.Println(campaigns, n, err)

	campaign := campaigns[0].Id

	res, n, err := NewAdGroupService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
		Predicates: []Predicate{
			Predicate{
				Field:    "CampaignId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(campaign, 10)},
			},
		},
	})

	fmt.Println(res, n, err)

	sharedsets, n, err := NewSharedSetService(&config.Auth).Get(Selector{
		Fields: []string{"SharedSetId", "Name", "Type"},
	})

	if err != nil {
		t.Error("sharedset: ", err)
	}

	sharedset := sharedsets[0].Id

	originalcrits, _, err := NewSharedCriterionService(&config.Auth).Get(Selector{
		Fields: []string{"SharedSetId", "Negative"},
		Predicates: []Predicate{
			Predicate{
				Field:    "SharedSetId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(sharedset, 10)},
			},
		},
	})
	if err != nil {
		t.Error(err)
	}

	config.Auth.ValidateOnly = true
	err = NewSharedCriterionService(&config.Auth).Mutate([]SharedCriterionOperation{
		{"ADD", SharedCriterion{
			SharedSetId: sharedset,
			Negative:    true,
			Criterion: KeywordCriterion{
				MatchType: "PHRASE",
				Text:      "bbbb",
			},
		}},
	})

	if err != nil {
		t.Error(err)
	}

	config.Auth.ValidateOnly = false
	currentcrits, _, err := NewSharedCriterionService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "SharedSetId", "Negative", "KeywordText"},
		Predicates: []Predicate{
			Predicate{
				Field:    "SharedSetId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(sharedset, 10)},
			},
		},
	})
	if err != nil {
		t.Error(err)
	}

	if len(originalcrits) != len(currentcrits) {
		t.Errorf("actual crits after validateonly mutate: %d, expected: %d\n", len(currentcrits), len(originalcrits))
	}
}

func TestSandboxSharedEntity(t *testing.T) {
	config := getTestConfig()

	campaigns, n, err := NewCampaignService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
	})

	fmt.Println(campaigns, n, err)

	campaign := campaigns[0].Id

	res, n, err := NewAdGroupService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
		Predicates: []Predicate{
			Predicate{
				Field:    "CampaignId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(campaign, 10)},
			},
		},
	})

	fmt.Println(res, n, err)

	/*
		err = NewSharedSetService(&config.Auth).Mutate([]SharedSetOperation{
			{"ADD", SharedSet{Name: "sharedset", Type: "NEGATIVE_KEYWORDS"}},
		})

	*/

	sharedsets, n, err := NewSharedSetService(&config.Auth).Get(Selector{
		Fields: []string{"SharedSetId", "Name", "Type"},
	})

	if err != nil {
		t.Error("sharedset: ", err)
	}

	fmt.Println(sharedsets)

	sharedset := sharedsets[0].Id

	err = NewSharedCriterionService(&config.Auth).Mutate([]SharedCriterionOperation{
		{"ADD", SharedCriterion{
			SharedSetId: sharedset,
			Negative:    true,
			Criterion: KeywordCriterion{
				MatchType: "PHRASE",
				Text:      "bbbb",
			},
		}},
	})

	if err != nil {
		t.Error(err)
	}

	err = NewCampaignSharedSetService(&config.Auth).Mutate([]CampaignSharedSetOperation{
		{"REMOVE", CampaignSharedSet{CampaignId: campaign, SharedSetId: sharedset}},
	})

	if err != nil {
		t.Error(err)
	}

	err = NewCampaignSharedSetService(&config.Auth).Mutate([]CampaignSharedSetOperation{
		{"ADD", CampaignSharedSet{CampaignId: campaign, SharedSetId: sharedset}},
	})

	if err != nil {
		t.Error(err)
	}

	sharedsetcrits, _, err := NewSharedCriterionService(&config.Auth).Get(Selector{
		Fields: []string{"SharedSetId", "Negative"},
		Predicates: []Predicate{
			Predicate{
				Field:    "SharedSetId",
				Operator: "EQUALS",
				Values:   []string{strconv.FormatInt(sharedset, 10)},
			},
		},
	})
	if err != nil {
		t.Error(err)
	}

	fmt.Println(sharedsetcrits)

	ss, _, err := NewCampaignSharedSetService(&config.Auth).Get(Selector{
		Fields: []string{"SharedSetId", "CampaignId", "SharedSetName"},
	})

	if err != nil {
		t.Error(err)
	}

	fmt.Println(ss)
}

func TestRateError(t *testing.T) {
	if e := os.Getenv("RUN_EXTRA_TESTS"); e == "" {
		t.Skip()
	}

	config := getTestConfig()
	wg := sync.WaitGroup{}

	for j := 0; j < 40; j++ {
		wg.Add(1)
		go func() {
			for i := 0; i < 100; i++ {
				_, _, err := NewCampaignService(&config.Auth).Get(Selector{
					Fields: []string{"Id", "Name", "CampaignId"},
				})

				if err == nil {
					continue
				}

				if err, ok := err.(Error); ok {
					fmt.Printf("%#v\n", err.OrigErr())
					if err.Code() != "RATE_EXCEEDED" {
						t.Fatalf("got %s error code, expected RATE_EXCEEDED\n", err.Code())
					}
				} else {
					t.Fatalf("expected error to fill Error interface\n")
				}

				t.Fatal()
			}
			wg.Done()
		}()
	}

	wg.Wait()

}

func TestAddSearchAdGroup(t *testing.T) {
	config := getTestConfig()

	campaigns, _, err := NewCampaignService(&config.Auth).Get(Selector{
		Fields: []string{"Id", "Name"},
	})
	if err != nil {
		t.Fatalf("didn't expect the get campaigns call to fail: %v", err)
	}

	newAdgroupName := fmt.Sprintf("test_adgroup_%d", time.Now().UnixNano())

	ops := make(map[string][]AdGroup)
	ops["ADD"] = []AdGroup{
		AdGroup{
			Name:         newAdgroupName,
			CampaignId:   campaigns[0].Id,
			Status:       "PAUSED",
			Labels:       make([]Label, 0),
			Type:         "SHOPPING_PRODUCT_ADS",
			RotationMode: "OPTIMIZE",
		},
	}
	adgroups, err := NewAdGroupService(&config.Auth).Mutate(ops)
	if err != nil {
		t.Fatalf("didn't expect an error creating adgroup: %v", err)
	}

	remove_ops := make(map[string][]AdGroup)
	remove_ops["SET"] = []AdGroup{
		AdGroup{
			Id:     adgroups[0].Id,
			Status: "REMOVED",
		},
	}
	_, err = NewAdGroupService(&config.Auth).Mutate(remove_ops)
	if err != nil {
		t.Fatalf("didn't expect the adgroup remove to fail: %v", err)
	}
}

type StringClient string

func (s StringClient) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{
		Body:       BufferCloser{bytes.NewBufferString(string(s))},
		StatusCode: http.StatusInternalServerError,
	}, nil
}

func TestSandboxEmptyErrorMessage(t *testing.T) {
	client := StringClient(`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><soap:Fault><faultcode>soap:Client</faultcode><faultstring>Unmarshalling Error: cvc-complex-type.2.4.a: Invalid content was found starting with element 'adServingOptimizationStatus'. One of '{"https://adwords.google.com/api/adwords/cm/v201802":status, "https://adwords.google.com/api/adwords/cm/v201802":settings, "https://adwords.google.com/api/adwords/cm/v201802":labels, "https://adwords.google.com/api/adwords/cm/v201802":forwardCompatibilityMap, "https://adwords.google.com/api/adwords/cm/v201802":biddingStrategyConfiguration, "https://adwords.google.com/api/adwords/cm/v201802":contentBidCriterionTypeGroup, "https://adwords.google.com/api/adwords/cm/v201802":baseCampaignId, "https://adwords.google.com/api/adwords/cm/v201802":baseAdGroupId, "https://adwords.google.com/api/adwords/cm/v201802":trackingUrlTemplate, "https://adwords.google.com/api/adwords/cm/v201802":urlCustomParameters, "https://adwords.google.com/api/adwords/cm/v201802":adGroupType, "https://adwords.google.com/api/adwords/cm/v201802":adGroupAdRotationMode}' is expected. </faultstring></soap:Fault></soap:Body></soap:Envelope>`)
	auth := &Auth{
		Client: client,
	}

	_, _, err := NewCampaignService(auth).Get(Selector{})

	if err == nil {
		t.Fatal("Test is not giving an error")
	}

	if err != nil && err.Error() == "" {
		t.Fatal("Test giving a blank error message")
	}
}
