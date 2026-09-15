import request from '../utils/request'
import type { FavoriteItem } from '../types'

export function addFavorite(productId: number) {
  return request.post<never, { code: number; message: string; data: FavoriteItem }>('/favorites', { product_id: productId })
}

export function removeFavorite(productId: number) {
  return request.delete<never, { code: number; message: string; data: { product_id: number } }>(`/favorites/${productId}`)
}

export function listMyFavorites(status?: string) {
  return request.get<never, { code: number; message: string; data: FavoriteItem[] }>('/favorites/me', {
    params: status ? { status } : {},
  })
}
