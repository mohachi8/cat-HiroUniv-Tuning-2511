-- このファイルに記述されたSQLコマンドが、マイグレーション時に実行されます。

CREATE INDEX idx_orders_user_id ON orders(user_id);

-- productsテーブルのパフォーマンス改善用インデックス
-- ソート用: ORDER BY句で使用されるvalue, weightとproduct_idの複合インデックス
-- これにより、ORDER BY value/weight + product_id ASCのソートが高速化される
CREATE INDEX idx_products_value_product_id ON products(value, product_id);
CREATE INDEX idx_products_weight_product_id ON products(weight, product_id);

-- 検索用: nameとdescriptionを結合した生成カラムを追加
-- このカラムにFULLTEXT INDEXを貼ることで、短い検索文字列でも高速化が可能
-- STORED型の生成カラムを使用することで、検索パフォーマンスを最大化
ALTER TABLE products 
ADD COLUMN search_text TEXT AS (CONCAT(COALESCE(name, ''), ' ', COALESCE(description, ''))) STORED;

-- 検索用: search_textカラムにFULLTEXT INDEX (N-gramパーサー)を追加
-- nameとdescriptionを結合したカラムに対してインデックスを貼ることで、
-- すべての検索文字列（1文字など短い文字列も含む）で高速化される
-- ngram_token_size=1の設定により、すべての検索文字列でインデックスが有効に機能する
-- COUNT(*)でも、FULLTEXT INDEXを使ってマッチする行だけをカウントできるため、全件スキャンは不要
ALTER TABLE products 
ADD FULLTEXT INDEX idx_products_search_text_ft (search_text) WITH PARSER ngram;