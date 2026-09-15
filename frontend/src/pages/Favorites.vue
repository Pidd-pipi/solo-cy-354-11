<template>
  <div class="page">
    <h2>我的收藏</h2>
    <el-tabs v-model="tab">
      <el-tab-pane v-for="t in tabs" :key="t.value" :label="`${t.label} (${countBy(t.value)})`" :name="t.value" />
    </el-tabs>
    <el-row :gutter="16">
      <el-col v-for="f in filtered" :key="f.id" :span="6" class="col">
        <el-card class="fav-card" shadow="hover">
          <div class="fav-head">
            <h3>{{ f.product?.title }}</h3>
            <el-tag :type="productStatusType(f.product?.status || '') as any" size="small">
              {{ productStatusLabel(f.product?.status || '') }}
            </el-tag>
          </div>
          <div class="fav-price">
            <span class="current">¥{{ f.current_price.toFixed(2) }}</span>
            <template v-if="f.price_dropped">
              <span class="origin">¥{{ f.price_at_favorite.toFixed(2) }}</span>
              <el-tag type="danger" size="small" effect="dark">已降价</el-tag>
            </template>
          </div>
          <div class="fav-meta">收藏时价格 ¥{{ f.price_at_favorite.toFixed(2) }} · {{ formatDateTime(f.created_at) }}</div>
          <div class="fav-actions">
            <el-button size="small" @click="showDetail(f)">详情</el-button>
            <el-button
              v-if="f.product?.status === 'on_sale'"
              size="small"
              type="primary"
              @click="buy(f)"
            >购买</el-button>
            <el-button v-if="f.product?.status === 'on_sale'" size="small" @click="chat(f)">私信</el-button>
            <el-button size="small" type="danger" plain @click="unfavorite(f)">取消收藏</el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>
    <el-empty v-if="!loading && filtered.length === 0" description="暂无收藏商品" />
    <el-dialog v-model="detailVisible" :title="current?.product?.title" width="520px">
      <el-descriptions :column="2" border v-if="current?.product">
        <el-descriptions-item label="分类">{{ categoryLabel(current.product.category) }}</el-descriptions-item>
        <el-descriptions-item label="成色">{{ current.product.condition }}</el-descriptions-item>
        <el-descriptions-item label="校区">{{ current.product.campus }}</el-descriptions-item>
        <el-descriptions-item label="交易地点">{{ current.product.trade_location }}</el-descriptions-item>
        <el-descriptions-item label="现价">¥{{ current.current_price.toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="收藏时价格">¥{{ current.price_at_favorite.toFixed(2) }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ productStatusLabel(current.product.status) }}</el-descriptions-item>
        <el-descriptions-item label="降价提醒">
          <el-tag v-if="current.price_dropped" type="danger" size="small">已降价</el-tag>
          <span v-else>无</span>
        </el-descriptions-item>
        <el-descriptions-item label="描述" :span="2">{{ current.product.description }}</el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { categoryLabel, productStatusLabel, productStatusType } from '../constants/product'
import { listMyFavorites, removeFavorite } from '../api/favorite'
import { createTradeOrder } from '../api/tradeOrder'
import { createConversation } from '../api/conversation'
import { formatDateTime } from '../utils/dateFormat'
import type { FavoriteItem } from '../types'
import { useRouter } from 'vue-router'

const router = useRouter()
const favorites = ref<FavoriteItem[]>([])
const loading = ref(false)
const tab = ref('all')
const detailVisible = ref(false)
const current = ref<FavoriteItem | null>(null)

const tabs = [
  { value: 'all', label: '全部' },
  { value: 'on_sale', label: '在售' },
  { value: 'removed', label: '已下架' },
  { value: 'sold', label: '已售出' },
]

const filtered = computed(() =>
  tab.value === 'all' ? favorites.value : favorites.value.filter((f) => f.product?.status === tab.value),
)

function countBy(value: string): number {
  return value === 'all' ? favorites.value.length : favorites.value.filter((f) => f.product?.status === value).length
}

async function load() {
  loading.value = true
  try {
    const res = await listMyFavorites()
    favorites.value = res.data
  } finally {
    loading.value = false
  }
}

function showDetail(f: FavoriteItem) {
  current.value = f
  detailVisible.value = true
}

async function buy(f: FavoriteItem) {
  await createTradeOrder(f.product_id)
  ElMessage.success('已下单，等待卖家确认')
}

async function chat(f: FavoriteItem) {
  await createConversation(f.product_id)
  ElMessage.success('已发起私信')
  router.push('/messages')
}

async function unfavorite(f: FavoriteItem) {
  await ElMessageBox.confirm(`确定取消收藏「${f.product?.title}」吗？`, '取消收藏', { type: 'warning' })
  await removeFavorite(f.product_id)
  favorites.value = favorites.value.filter((x) => x.id !== f.id)
  ElMessage.success('已取消收藏')
}

onMounted(load)
</script>

<style scoped>
.col {
  margin-bottom: 16px;
}
.fav-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}
.fav-head h3 {
  margin: 0;
  font-size: 16px;
  color: #303133;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}
.fav-price {
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: 12px 0 4px;
}
.fav-price .current {
  color: #f56c6c;
  font-size: 20px;
  font-weight: 700;
}
.fav-price .origin {
  color: #909399;
  font-size: 13px;
  text-decoration: line-through;
}
.fav-meta {
  color: #909399;
  font-size: 12px;
  margin-bottom: 12px;
}
.fav-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
</style>
