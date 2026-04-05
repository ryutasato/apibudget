#!/bin/bash
# Apply EVERYTHING FROM SCRATCH cleanly

# memory_store_test.go
sed -i 's/	store.IncrementWindow(ctx, "key1", 5, 10\*time.Second)/	_, _ = store.IncrementWindow(ctx, "key1", 5, 10*time.Second)/' memory_store_test.go
sed -i 's/	store.IncrementWindow(ctx, "key1", 10, 10\*time.Second)/	_, _ = store.IncrementWindow(ctx, "key1", 10, 10*time.Second)/' memory_store_test.go
sed -i 's/	store.IncrementWindow(ctx, "key1", 2, 10\*time.Second)/	_, _ = store.IncrementWindow(ctx, "key1", 2, 10*time.Second)/' memory_store_test.go
sed -i 's/	store.IncrementWindow(ctx, "key1", 5, 50\*time.Millisecond)/	_, _ = store.IncrementWindow(ctx, "key1", 5, 50*time.Millisecond)/' memory_store_test.go
sed -i 's/	store.SetCredit(ctx, "pool1", MustNewCredit("100"))/	_ = store.SetCredit(ctx, "pool1", MustNewCredit("100"))/' memory_store_test.go
sed -i 's/	store.SetCredit(ctx, "pool1", MustNewCredit("10"))/	_ = store.SetCredit(ctx, "pool1", MustNewCredit("10"))/' memory_store_test.go
sed -i 's/	store.SetCredit(ctx, "pool1", MustNewCredit("50"))/	_ = store.SetCredit(ctx, "pool1", MustNewCredit("50"))/' memory_store_test.go

# Fix SA1012 in manager_test.go
sed -i 's/nil, "pool1"/context.Background(), "pool1"/g' manager_test.go
sed -i 's/nil, "pool_a"/context.Background(), "pool_a"/g' manager_test.go
sed -i 's/nil, "pool_b"/context.Background(), "pool_b"/g' manager_test.go
sed -i 's/nil, "prop4_pool"/context.Background(), "prop4_pool"/g' manager_test.go
sed -i 's/nil, "prop7_pool"/context.Background(), "prop7_pool"/g' manager_test.go
sed -i 's/nil, "prop8_pool"/context.Background(), "prop8_pool"/g' manager_test.go
sed -i 's/nil, "prop9_pool"/context.Background(), "prop9_pool"/g' manager_test.go
sed -i 's/nil, "prop10_pool"/context.Background(), "prop10_pool"/g' manager_test.go
sed -i 's/nil, "prop11_pool"/context.Background(), "prop11_pool"/g' manager_test.go
sed -i 's/nil, "prop12_pool"/context.Background(), "prop12_pool"/g' manager_test.go
sed -i 's/nil, "prop24_pool"/context.Background(), "prop24_pool"/g' manager_test.go
sed -i 's/nil, "dl_pool"/context.Background(), "dl_pool"/g' manager_test.go
sed -i 's/nil, key/context.Background(), key/g' manager_test.go

# Fix manager_test.go gosimple issue manually
sed -i '2285,2289c\
		return tokensD == tokensE' manager_test.go

# shadow errors
sed -i '257s/err := r.Confirm/err2 := r.Confirm/' concurrency_test.go
sed -i '258s/if err != nil/if err2 != nil/' concurrency_test.go
sed -i '259s/err, err)/err2, err2)/' concurrency_test.go

sed -i '1285s/if err := m.ResetCredits/if err2 := m.ResetCredits/' manager_test.go
sed -i '1285s/; err != nil/; err2 != nil/' manager_test.go
sed -i '1286s/err)/err2)/' manager_test.go

sed -i '1505s/if err := m.ResetCredits/if err2 := m.ResetCredits/' manager_test.go
sed -i '1505s/; err != nil/; err2 != nil/' manager_test.go
sed -i '1506s/err)/err2)/' manager_test.go

sed -i '1556s/if err := m.AddCredits/if err2 := m.AddCredits/' manager_test.go
sed -i '1556s/; err != nil/; err2 != nil/' manager_test.go
sed -i '1557s/err)/err2)/' manager_test.go

sed -i '1602s/if err := m.SetCredits/if err2 := m.SetCredits/' manager_test.go
sed -i '1602s/; err != nil/; err2 != nil/' manager_test.go
sed -i '1603s/err)/err2)/' manager_test.go

sed -i '50s/if err := store.SetCredit/if err2 := store.SetCredit/' memory_store_test.go
sed -i '50s/; err != nil/; err2 != nil/' memory_store_test.go
sed -i '51s/err)/err2)/' memory_store_test.go

# redis_store_test.go
sed -i '29s/store.Close()/_ = store.Close()/' redis_store_test.go
sed -i '338s/store.Close()/_ = store.Close()/' redis_store_test.go
sed -i '31s/return store.(\*redisStore)/res, _ := store.(*redisStore)\n\treturn res/' redis_store_test.go

# server_test.go
sed -i '435s/json.NewDecoder(rec.Body).Decode(&resResp)/_ = json.NewDecoder(rec.Body).Decode(\&resResp)/' server_test.go

# server.go
sed -i '430s/json.NewEncoder(w).Encode(v)/_ = json.NewEncoder(w).Encode(v)/' server.go

# main.go
sed -i 's/defer store.Close()/defer func() { _ = store.Close() }()/' cmd/apibudget-server/main.go

# Misspell fixes EXCEPT the server.go/server_test.go ones
sed -i 's/cancelled/canceled/g' reservation.go
sed -i 's/cancelled/canceled/g' manager_test.go
gofmt -w reservation.go
