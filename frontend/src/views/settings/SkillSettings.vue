<!-- AI-generated: Skills management panel. Lists preloaded skills; click a card to view its SKILL.md content in a dialog. -->
<template>
  <div class="skills-settings">
    <div class="section-header">
      <h2>{{ $t('skillsSettings.title') }}</h2>
      <p class="section-description">{{ $t('skillsSettings.description') }}</p>
    </div>

    <div v-if="loading" class="loading-container">
      <t-loading :text="$t('common.loading')" />
    </div>

    <template v-else-if="!skillsAvailable">
      <div class="empty-state">
        <t-empty :description="$t('skillsSettings.sandboxDisabled')" />
      </div>
    </template>

    <template v-else>
      <div class="list-section-header">
        <h3>{{ $t('skillsSettings.existingSkills') }}</h3>
        <p>{{ $t('skillsSettings.manageHint') }}</p>
      </div>

      <div v-if="skills.length === 0" class="empty-state">
        <t-empty :description="$t('skillsSettings.empty')" />
      </div>

      <div v-else class="skills-grid">
        <div
          v-for="skill in skills"
          :key="skill.name"
          class="skill-card"
          role="button"
          tabindex="0"
          :title="$t('skillsSettings.viewContent')"
          @click="openSkill(skill)"
          @keydown.enter.prevent="openSkill(skill)"
          @keydown.space.prevent="openSkill(skill)"
        >
          <div class="skill-card__badge"><t-icon name="code-1" size="18px" /></div>
          <div class="skill-card__body">
            <h3 class="skill-card__title" :title="skill.name">{{ skill.name }}</h3>
            <div class="skill-card__desc" :title="skill.description">{{ skill.description }}</div>
          </div>
          <t-icon name="chevron-right" size="16px" class="skill-card__arrow" />
        </div>
      </div>
    </template>

    <!-- Skill 内容查看弹窗 -->
    <t-dialog
      v-model:visible="detailVisible"
      :header="detailTitle"
      :footer="false"
      width="760px"
      top="8vh"
      dialog-class-name="skill-detail-dialog"
      destroy-on-close
      @close="handleCloseDetail"
    >
      <div class="skill-detail">
        <div v-if="detailLoading" class="skill-detail__loading">
          <t-loading :text="$t('common.loading')" />
        </div>

        <div v-else-if="detailError" class="skill-detail__error">
          <t-empty :description="$t('skillsSettings.toasts.loadDetailFailed')" />
        </div>

        <template v-else>
          <p v-if="detailDescription" class="skill-detail__desc">{{ detailDescription }}</p>
          <div
            v-if="skillContentHtml"
            class="skill-detail__content markdown-content"
            v-html="skillContentHtml"
          ></div>
          <t-empty v-else :description="$t('skillsSettings.noContent')" />
        </template>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { marked } from 'marked'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { listSkills, getSkill, type SkillInfo, type SkillDetail } from '@/api/skill'
import { sanitizeMarkdownHTML } from '@/utils/security'
import { configureMarkedForChatMarkdown } from '@/utils/chatMarkdownRenderer'

configureMarkedForChatMarkdown()

const { t } = useI18n()
const skills = ref<SkillInfo[]>([])
const skillsAvailable = ref(true)
const loading = ref(false)

const loadSkills = async () => {
  loading.value = true
  try {
    const res = await listSkills()
    skillsAvailable.value = res.skills_available !== false
    skills.value = res.data && res.data.length > 0 ? res.data : []
  } catch (error) {
    skillsAvailable.value = false
    skills.value = []
    MessagePlugin.error(t('skillsSettings.toasts.loadFailed'))
    console.error('Failed to load skills:', error)
  } finally {
    loading.value = false
  }
}

// ---- Skill 详情弹窗 ----
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailError = ref(false)
const detailName = ref('')
const detailDescription = ref('')
const detailInstructions = ref('')

const detailTitle = computed(() => detailName.value || t('skillsSettings.title'))

const skillContentHtml = computed(() => {
  const md = detailInstructions.value
  if (!md) return ''
  try {
    const raw = String(marked.parse(md))
    return sanitizeMarkdownHTML(raw)
  } catch (error) {
    console.error('Failed to render skill markdown:', error)
    return ''
  }
})

const openSkill = async (skill: SkillInfo) => {
  detailName.value = skill.name
  detailDescription.value = skill.description
  detailInstructions.value = ''
  detailError.value = false
  detailVisible.value = true
  detailLoading.value = true
  try {
    const res = await getSkill(skill.name)
    const detail: SkillDetail | undefined = res?.data
    if (detail) {
      detailDescription.value = detail.description || skill.description
      detailInstructions.value = detail.instructions || ''
    }
  } catch (error) {
    detailError.value = true
    MessagePlugin.error(t('skillsSettings.toasts.loadDetailFailed'))
    console.error('Failed to load skill detail:', error)
  } finally {
    detailLoading.value = false
  }
}

const handleCloseDetail = () => {
  detailLoading.value = false
  detailError.value = false
  detailInstructions.value = ''
}

onMounted(() => {
  loadSkills()
})
</script>

<style scoped lang="less">
@import '../../components/css/chat-markdown.less';

.skills-settings {
  width: 100%;
}

.section-header {
  margin-bottom: 28px;

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 8px 0;
  }

  .section-description {
    font-size: 14px;
    color: var(--td-text-color-secondary);
    margin: 0;
    line-height: 1.6;
  }
}

.loading-container {
  padding: 40px 0;
  text-align: center;
}

.list-section-header {
  margin-bottom: 16px;

  h3 {
    font-size: 16px;
    font-weight: 600;
    color: var(--td-text-color-primary);
    margin: 0 0 4px 0;
  }

  p {
    font-size: 13px;
    color: var(--td-text-color-placeholder);
    margin: 0;
    line-height: 1.5;
  }
}

.empty-state {
  padding: 80px 0;
  text-align: center;

  :deep(.t-empty__description) {
    font-size: 14px;
    color: var(--td-text-color-placeholder);
    margin-bottom: 16px;
  }
}

.skills-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 12px;
}

.skill-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 14px 14px 14px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  min-width: 0;
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 2px 12px rgba(0, 82, 217, 0.12);
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

.skill-card__badge {
  flex-shrink: 0;
  width: 36px;
  height: 36px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-top: 1px;
  background: rgba(0, 82, 217, 0.1);
  color: #0052D9;
}

.skill-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.skill-card__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.4;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.skill-card__desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.skill-card__arrow {
  flex-shrink: 0;
  margin-top: 10px;
  color: var(--td-text-color-placeholder);
}

// ---- 详情弹窗 ----
.skill-detail {
  min-height: 120px;
}

.skill-detail__loading,
.skill-detail__error {
  padding: 48px 0;
  text-align: center;
}

.skill-detail__desc {
  margin: 0 0 16px 0;
  padding: 10px 12px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 8px;
}

.skill-detail__content {
  padding-right: 4px;
  .chat-markdown-typography();
}
</style>

<style lang="less">
// Dialog 被 teleport 到 body，scoped 样式够不着，需用非 scoped 样式约束。
// 注意：dialogClassName 是加在 .t-dialog 元素自身上（与 t-dialog 同一元素），
// 所以直接选中 .skill-detail-dialog，而不是 .skill-detail-dialog .t-dialog。
.skill-detail-dialog {
  max-height: 84vh;
  display: flex;
  flex-direction: column;

  .t-dialog__body {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
  }
}
</style>

