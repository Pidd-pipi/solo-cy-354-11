<template>
  <div class="page">
    <h2>个人中心</h2>
    <template v-if="authStore.user">
      <el-card class="profile-card">
        <div class="profile-head">
          <el-avatar :size="64" :src="authStore.user.avatar || ''">{{ authStore.user.nickname.slice(0, 1) }}</el-avatar>
          <div class="profile-info">
            <h3>{{ authStore.user.nickname }}</h3>
            <p>{{ authStore.user.phone }} · {{ roleLabel(authStore.user.role) }} · {{ authStore.user.campus }}</p>
          </div>
          <div class="credit-box">
            <div class="credit-label">信誉分</div>
            <div class="credit-value">{{ authStore.user.credit_score }}</div>
            <div class="credit-level">{{ creditLevel(authStore.user.credit_score) }}</div>
          </div>
        </div>
      </el-card>
      <el-card class="section">
        <template #header>🛍️ 我发布的商品</template>
        <el-table :data="myProducts">
          <el-table-column prop="title" label="标题" />
          <el-table-column label="价格" width="120">
            <template #default="{ row }">¥{{ row.price.toFixed(2) }}</template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="productStatusType(row.status) as any">{{ productStatusLabel(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120">
            <template #default="{ row }">
              <el-button size="small" :disabled="row.status !== 'on_sale'" @click="openPriceDialog(row)">改价</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
      <el-card class="section">
        <template #header>⭐ 收到的评价</template>
        <el-table :data="reviews">
          <el-table-column prop="id" label="ID" width="80" />
          <el-table-column label="评价">
            <template #default="{ row }">
              <el-tag size="small">{{ ratingLabel(row.rating) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="content" label="内容" />
          <el-table-column label="时间">
            <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>
    <el-empty v-else description="请先登录">
      <el-button type="primary" @click="$router.push('/login')">去登录</el-button>
    </el-empty>
    <el-dialog v-model="priceDialogVisible" title="修改价格" width="360px">
      <el-form label-width="80px">
        <el-form-item label="商品">
          <span>{{ editingProduct?.title }}</span>
        </el-form-item>
        <el-form-item label="新价格">
          <el-input-number v-model="newPrice" :min="0.01" :precision="2" :step="1" style="width: 180px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="priceDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitPrice">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '../stores/authStore'
import { roleLabel } from '../constants/user'
import { ratingLabel } from '../constants/trade'
import { productStatusLabel, productStatusType } from '../constants/product'
import { listMyReviews } from '../api/review'
import { listProducts, updateProductPrice } from '../api/product'
import { formatDateTime } from '../utils/dateFormat'
import type { Product, Review } from '../types'

const authStore = useAuthStore()
const reviews = ref<Review[]>([])
const myProducts = ref<Product[]>([])
const priceDialogVisible = ref(false)
const editingProduct = ref<Product | null>(null)
const newPrice = ref(0)

function creditLevel(score: number): string {
  if (score >= 200) return '极佳'
  if (score >= 150) return '优秀'
  if (score >= 100) return '良好'
  if (score >= 60) return '一般'
  return '待提升'
}

function openPriceDialog(p: Product) {
  editingProduct.value = p
  newPrice.value = p.price
  priceDialogVisible.value = true
}

async function submitPrice() {
  if (!editingProduct.value) return
  await updateProductPrice(editingProduct.value.id, newPrice.value)
  editingProduct.value.price = newPrice.value
  priceDialogVisible.value = false
  ElMessage.success('价格已更新')
}

onMounted(async () => {
  if (!authStore.token) return
  const res = await listMyReviews()
  reviews.value = res.data
  if (authStore.user) {
    const mine = await listProducts({ seller_id: authStore.user.id, page_size: 100 })
    myProducts.value = mine.data.items
  }
})
</script>

<style scoped>
.profile-card {
  margin-bottom: 16px;
}
.profile-head {
  display: flex;
  align-items: center;
  gap: 16px;
}
.profile-info h3 {
  margin: 0;
}
.profile-info p {
  margin: 4px 0 0;
  color: #909399;
  font-size: 13px;
}
.credit-box {
  margin-left: auto;
  text-align: center;
}
.credit-label {
  color: #909399;
  font-size: 12px;
}
.credit-value {
  font-size: 28px;
  font-weight: 700;
  color: #e6a23c;
}
.credit-level {
  font-size: 12px;
  color: #909399;
}
.section {
  margin-bottom: 16px;
}
</style>
