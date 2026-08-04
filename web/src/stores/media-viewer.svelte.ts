import type { DetailAttachment } from '../lib/types'

class MediaViewerStore {
  attachment = $state<DetailAttachment | null>(null)

  open(attachment: DetailAttachment): void {
    this.attachment = attachment
  }

  close(): void {
    this.attachment = null
  }
}

export const mediaViewer = new MediaViewerStore()
