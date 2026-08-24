#!/usr/bin/env bash
# ==============================================================================
# Service Hub - 流水线任务模拟与并发流量生成脚本
# Simulate pipeline task dispatch to generate metrics for Grafana dashboard
# ==============================================================================
set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

HUB_URL="${SERVICE_HUB_URL:-http://127.0.0.1:8082}"
COUNT="${1:-20}"

echo "========================================================"
echo " 🚀 Service Hub 流水线调度模拟器"
echo " 目标地址: $HUB_URL"
echo " 模拟批次: $COUNT 个任务"
echo "========================================================"

# 检查服务健康
if ! curl -s -f "$HUB_URL/api/health" > /dev/null; then
  echo "❌ 错误: Service Hub 未在 $HUB_URL 运行，请先启动服务！"
  echo "💡 提示: bash ./scripts/dev/dev-start-new-modules.sh 或 cd services/service-hub && bash run.sh"
  exit 1
fi

echo -e "${BLUE}[*] 开始向调度中枢注入并发任务流...${NC}"

# 模拟 1: 敏感分类与自动脱敏分发 (/api/hub/classify)
for ((i=1; i<=COUNT; i++)); do
  PAYLOAD="{\"source\":\"dept_hospital_test\",\"operation\":\"auto_desensitize\",\"priority\":$((i%3+1)),\"payload\":{\"patient_id\":\"P00$i\",\"name\":\"张测试$i\",\"id_card\":\"51010419900101123$((i%10))\",\"diagnosis\":\"急性胃肠炎\",\"medical_fee\":$((100+i*20))}}"
  
  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$HUB_URL/api/hub/classify" \
    -H "Content-Type: application/json" \
    -d "$PAYLOAD" || echo "000")
  
  if [ "$HTTP_CODE" = "200" ]; then
    printf "${GREEN}✓${NC}"
  else
    printf "x"
  fi
  
  # 穿插普通任务分发 (/api/hub/dispatch)
  if (( i % 3 == 0 )); then
    curl -s -o /dev/null -X POST "$HUB_URL/api/hub/dispatch" \
      -H "Content-Type: application/json" \
      -d "{\"source\":\"yibao_settlement\",\"operation\":\"mask_id\",\"priority\":2,\"payload\":{\"record_id\":\"REC$i\"}}" || true
  fi
  
  # 穿插查询状态 (/api/hub/status)
  if (( i % 5 == 0 )); then
    curl -s -o /dev/null "$HUB_URL/api/hub/status" || true
    curl -s -o /dev/null "$HUB_URL/api/hub/tasks?limit=10" || true
  fi
  
  sleep 0.1
done

echo ""
echo "--------------------------------------------------------"
echo -e "${GREEN}✅ 模拟任务注入完成！${NC}"
echo "📊 请前往 Grafana (http://localhost:3000) 查看 Service Hub 调度监控大屏！"
echo "========================================================"
