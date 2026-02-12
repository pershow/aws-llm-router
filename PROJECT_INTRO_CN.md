# AWS Cursor Router 椤圭洰绠€浠?
## 椤圭洰姒傝堪

**AWS Cursor Router** 鏄竴涓珮鎬ц兘鐨?OpenAI 鍏煎浠ｇ悊鏈嶅姟鍣紝涓撲负 Cursor IDE 涓?AWS Bedrock 鐨勯泦鎴愯€岃璁°€傚畠浣滀负涓棿灞備唬鐞嗭紝浣垮洟闃熸垚鍛樿兘澶熼€氳繃缁熶竴鐨?API 瀵嗛挜瀹夊叏鍦颁娇鐢?AWS Bedrock 鐨勫ぇ璇█妯″瀷鏈嶅姟锛岃€屾棤闇€鐩存帴鎺ヨЕ AWS 鍑瘉銆?
## 鏍稿績鍔熻兘

### 1. OpenAI 鍏煎鎺ュ彛
- 瀹屽叏鍏煎 OpenAI API 瑙勮寖锛屾敮鎸?`/v1/chat/completions` 鍜?`/v1/responses` 绔偣
- 鏃犵紳闆嗘垚 Cursor IDE 鍙婂叾浠栨敮鎸?OpenAI API 鐨勫伐鍏?- 鏀寔娴佸紡锛坰treaming锛夊拰闈炴祦寮忓搷搴?
### 2. AWS Bedrock 浠ｇ悊
- 灏?OpenAI 鏍煎紡鐨勮姹傝浆鎹负 AWS Bedrock API 璋冪敤
- 鏀寔澶氱 Bedrock 妯″瀷锛堝 Claude 3.5 Sonnet 绛夛級
- AWS 鍑瘉闆嗕腑绠＄悊锛屽洟闃熸垚鍛樻棤闇€閰嶇疆 AWS 瀵嗛挜

### 3. 鏅鸿兘宸ュ叿璋冪敤鏀寔
- 瀹屾暣鏀寔鐜颁唬 AI 缂栫爜鍔╂墜鐨勫伐鍏疯皟鐢ㄦ祦绋?- 鏀寔 `tools`銆乣tool_choice`銆乣tool_calls` 绛夊弬鏁?- 鏀寔 `developer` 瑙掕壊锛堟槧灏勫埌绯荤粺鎻愮ず锛?- 鍙厤缃己鍒跺伐鍏蜂娇鐢ㄦā寮忥紙`FORCE_TOOL_USE`锛?- 鏀寔宸ュ叿鍙傛暟缂撳啿鏈哄埗锛岄伩鍏?JSON 鎴柇

### 4. 澶氱鎴风鐞?- 鍩轰簬 API Key 鐨勫鎴风璁よ瘉
- 姣忎釜瀹㈡埛绔彲閰嶇疆锛?  - 璇锋眰棰戠巼闄愬埗锛圧PM锛?  - 骞跺彂璇锋眰闄愬埗
  - 鍏佽浣跨敤鐨勬ā鍨嬪垪琛?  - 鍚敤/绂佺敤鐘舵€?- SQLite 鏁版嵁搴撴寔涔呭寲閰嶇疆鍜屾棩蹇?
### 5. 璇锋眰鐩戞帶涓庢棩蹇?- 璇︾粏鐨勮姹?鍝嶅簲鏃ュ織璁板綍
- 鏀寔宸ュ叿璋冪敤杩囩▼鐨勫畬鏁存棩蹇楄拷韪?- 鍙厤缃殑璋冭瘯妯″紡锛坄DEBUG_REQUESTS`锛?- 鍋ュ悍妫€鏌ョ鐐癸紙`/healthz`锛?
### 6. 绠＄悊鍚庡彴
- Web 绠＄悊鐣岄潰锛堣闂矾寰勶細`/salessavvy/`锛?- 鏀寔鍔ㄦ€侀厤缃?AWS 鍑瘉
- 瀹㈡埛绔鐞嗭紙娣诲姞銆佺紪杈戙€佸垹闄わ級
- 妯″瀷鍚敤/绂佺敤鎺у埗
- 璋冪敤鏃ュ織鏌ョ湅

### 7. 鐏垫椿閮ㄧ讲
- 鏀寔鏈湴鐩存帴杩愯锛坄go run`锛?- 鏀寔 Docker 瀹瑰櫒鍖栭儴缃?- 鏀寔 Docker Compose 涓€閿儴缃?- 鍙€夌殑 TLS 鍙嶅悜浠ｇ悊鍔熻兘

## 鎶€鏈灦鏋?
### 鎶€鏈爤
- **璇█**: Go 1.25.7
- **鏁版嵁搴?*: SQLite锛坢odernc.org/sqlite锛?- **AWS SDK**: aws-sdk-go-v2
- **鍏抽敭渚濊禆**:
  - `github.com/aws/aws-sdk-go-v2/service/bedrockruntime` - Bedrock 杩愯鏃惰皟鐢?  - `github.com/google/uuid` - UUID 鐢熸垚
  - `golang.org/x/time` - 閫熺巼闄愬埗

### 鏍稿績妯″潡

```
aws-cursor-router/
鈹溾攢鈹€ cmd/server/          # 涓荤▼搴忓叆鍙ｅ拰璺敱瀹氫箟
鈹溾攢鈹€ internal/
鈹?  鈹溾攢鈹€ auth/           # API Key 璁よ瘉鍜屽鎴风绠＄悊
鈹?  鈹溾攢鈹€ bedrockproxy/   # Bedrock API 浠ｇ悊鏈嶅姟
鈹?  鈹溾攢鈹€ config/         # 閰嶇疆鍔犺浇鍜岀鐞?鈹?  鈹溾攢鈹€ openai/         # OpenAI 鍗忚鏁版嵁缁撴瀯
鈹?  鈹斺攢鈹€ store/          # SQLite 鏁版嵁瀛樺偍灞?鈹溾攢鈹€ web/admin/          # 绠＄悊鍚庡彴闈欐€佹枃浠讹紙宓屽叆寮忥級
鈹斺攢鈹€ data/               # 鏁版嵁鐩綍锛圫QLite 鏁版嵁搴擄級
```

## 涓昏鐗规€?
### 瀹夊叏鎬?- AWS 鍑瘉鏈嶅姟鍣ㄧ闆嗕腑绠＄悊
- 鍩轰簬 API Key 鐨勮闂帶鍒?- 鏀寔瀹㈡埛绔骇鍒殑鏉冮檺闅旂

### 鎬ц兘
- 楂樺苟鍙戞敮鎸侊紙榛樿 512 骞跺彂锛?- 娴佸紡鍝嶅簲浼樺寲
- 璇锋眰棰戠巼闄愬埗鍜屽苟鍙戞帶鍒?
### 鍙厤缃€?- 鐜鍙橀噺閰嶇疆鏀寔
- 杩愯鏃跺姩鎬侀厤缃洿鏂?- 鐏垫椿鐨勬ā鍨嬮€夋嫨鍜?token 闄愬埗

### 鍏煎鎬?- 瀹屾暣鏀寔 Cursor Agent 妯″紡
- 鍏煎鏍囧噯 OpenAI SDK
- 鏀寔 CLI/IDE 闆嗘垚鐨?MCP/skills 椋庢牸宸ュ叿鎵ц

## 鍏稿瀷浣跨敤鍦烘櫙

1. **鍥㈤槦 AI 缂栫爜鍗忎綔**
   - 缁熶竴绠＄悊 AWS Bedrock 璁块棶鏉冮檺
   - 涓轰笉鍚屽洟闃熸垚鍛樺垎閰嶇嫭绔嬬殑 API Key
   - 鐩戞帶鍜岄檺鍒跺洟闃熸垚鍛樼殑浣跨敤閲?
2. **Cursor IDE 澧炲己**
   - 浣跨敤 AWS Bedrock 鐨勯珮鎬ц兘妯″瀷鏇夸唬 OpenAI
   - 鏀寔 Cursor Agent 妯″紡鐨勫畬鏁村姛鑳?   - 宸ュ叿璋冪敤鍜屼唬鐮佺紪杈戠殑娴佺晠浣撻獙

3. **鎴愭湰鎺у埗**
   - 闆嗕腑绠＄悊 AWS 璧勬簮
   - 闃叉 AWS 鍑瘉娉勯湶
   - 鐩戞帶鍜屽垎鏋?API 璋冪敤鎴愭湰

4. **寮€鍙戞祴璇?*
   - 蹇€熷垏鎹笉鍚岀殑 Bedrock 妯″瀷
   - 璋冭瘯 AI 宸ュ叿璋冪敤娴佺▼
   - 鏃ュ織鍒嗘瀽鍜岄棶棰樻帓鏌?
## 閰嶇疆绀轰緥

### Cursor IDE 閰嶇疆
```
Base URL: http://your-server:8080/v1
API Key: your-client-api-key
Model: anthropic.claude-3-5-sonnet-20240620-v1:0
```

### 鐜鍙橀噺閰嶇疆
```env
LISTEN_ADDR=:8080
AWS_REGION=us-east-1
DEFAULT_MODEL_ID=anthropic.claude-3-5-sonnet-20240620-v1:0
FORCE_TOOL_USE=true
MIN_TOOL_MAX_OUTPUT_TOKENS=8192
DEBUG_REQUESTS=true
```

## 蹇€熷紑濮?
### 鏈湴杩愯
```powershell
# 1. 澶嶅埗閰嶇疆鏂囦欢
Copy-Item .env.example .env

# 2. 缂栬緫 .env 閰嶇疆 AWS 鍑瘉

# 3. 瀹夎渚濊禆骞惰繍琛?go mod tidy
go run ./cmd/server

# 4. 鍋ュ悍妫€鏌?curl http://127.0.0.1:8080/healthz
```

### Docker 閮ㄧ讲
```bash
# 浣跨敤 Docker Compose
docker compose up -d --build

# 鏌ョ湅鏃ュ織
docker compose logs -f
```

## 椤圭洰浼樺娍

1. **寮€绠卞嵆鐢?*: 绠€鍗曢厤缃嵆鍙揩閫熼儴缃?2. **鐢熶骇灏辩华**: 瀹屽杽鐨勯敊璇鐞嗗拰鏃ュ織绯荤粺
3. **楂樻€ц兘**: Go 璇█瀹炵幇锛屼綆寤惰繜楂樺悶鍚?4. **鏄撲簬缁存姢**: 娓呮櫚鐨勬ā鍧楀寲鏋舵瀯
5. **鍔熻兘瀹屾暣**: 鏀寔鐜颁唬 AI 鍔╂墜鐨勬墍鏈夋牳蹇冨姛鑳?6. **鏂囨。榻愬叏**: 鎻愪緵璇︾粏鐨勬晠闅滄帓鏌ユ寚鍗楋紙TROUBLESHOOTING.md锛?
## 閫傜敤瀵硅薄

- 浣跨敤 Cursor IDE 鐨勫紑鍙戝洟闃?- 闇€瑕佺粺涓€绠＄悊 AWS Bedrock 璁块棶鐨勪紒涓?- 甯屾湜浣跨敤 AWS 妯″瀷浣嗛渶瑕?OpenAI 鍏煎鎺ュ彛鐨勫紑鍙戣€?- 闇€瑕佺洃鎺у拰鎺у埗 AI API 浣跨敤鐨勯」鐩粡鐞?
---

**椤圭洰绫诲瀷**: 浼佷笟绾?AI 浠ｇ悊鏈嶅姟鍣? 
**寮€鍙戣瑷€**: Go  
**寮€婧愬崗璁?*: 锛堣鏍规嵁瀹為檯鎯呭喌娣诲姞锛? 
**缁存姢鐘舵€?*: 娲昏穬寮€鍙戜腑