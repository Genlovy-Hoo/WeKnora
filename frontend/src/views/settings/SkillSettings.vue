<!-- AI-generated: Skills management panel. Click a skill card to open a dialog with a left file-tree and a right content viewer. -->
<template>
  <div class="skills-settings">
    <div v-if="!embedded" class="section-header">
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
      <!-- 第一层：Skill 库列表 -->
      <template v-if="!currentLibrary">
        <div class="list-section-header">
          <h3>{{ $t('skillsSettings.libraries') }}</h3>
          <p>{{ $t('skillsSettings.librariesHint') }}</p>
        </div>

        <div class="libraries-grid">
          <div
            v-for="library in libraries"
            :key="library.name"
            class="library-card"
            role="button"
            tabindex="0"
            :title="$t('skillsSettings.openLibrary')"
            @click="openLibrary(library.name)"
            @keydown.enter.prevent="openLibrary(library.name)"
            @keydown.space.prevent="openLibrary(library.name)"
          >
            <div class="library-card__badge"><t-icon name="folder" size="20px" /></div>
            <div class="library-card__body">
              <h3 class="library-card__title" :title="library.name">{{ library.name }}</h3>
              <div class="library-card__desc" :title="library.description || ''">
                {{ library.description || $t('skillsSettings.libraryNoDesc') }}
              </div>
              <div class="library-card__count">{{ (library.skills || []).length }} {{ $t('skillsSettings.skillsUnit') }}</div>
            </div>
            <t-icon name="chevron-right" size="18px" class="library-card__arrow" />

            <!-- 右上角操作：修改 / 删除 -->
            <div class="library-card__actions" @click.stop @keydown.stop>
              <t-dropdown :options="libraryActionOptions" trigger="click" @click="(opt: any) => onLibraryAction(opt, library)">
                <t-button theme="default" variant="text" size="small" class="library-card__actions-btn">
                  {{ $t('skillsSettings.actions') }}
                  <template #suffix><t-icon name="chevron-down" size="14px" /></template>
                </t-button>
              </t-dropdown>
            </div>
          </div>

          <!-- 创建 Skill 库 -->
          <div
            class="library-card library-card--create"
            role="button"
            tabindex="0"
            :title="$t('skillsSettings.createLibrary')"
            @click="openCreateDialog"
            @keydown.enter.prevent="openCreateDialog"
            @keydown.space.prevent="openCreateDialog"
          >
            <div class="library-card__badge library-card__badge--create"><t-icon name="add" size="20px" /></div>
            <div class="library-card__body">
              <h3 class="library-card__title">{{ $t('skillsSettings.createLibrary') }}</h3>
              <div class="library-card__desc">{{ $t('skillsSettings.createLibraryHint') }}</div>
            </div>
          </div>
        </div>

        <div v-if="libraries.length === 0" class="empty-state">
          <t-empty :description="$t('skillsSettings.emptyLibraries')" />
        </div>
      </template>

      <!-- 第二层：某 Skill 库内的 skills 列表 -->
      <template v-else>
        <div class="list-section-header list-section-header--with-back">
          <div class="list-section-header__text">
            <h3>{{ currentLibrary }}</h3>
            <p>{{ $t('skillsSettings.manageHint') }}</p>
          </div>
          <div class="list-section-header__actions">
            <t-button theme="default" variant="text" class="back-to-libraries-btn" @click="backToLibraries">
              <template #icon><t-icon name="arrow-left" /></template>
              {{ $t('skillsSettings.backToLibraries') }}
            </t-button>
          </div>
        </div>

        <div v-if="displayedSkills.length === 0" class="empty-state">
          <t-empty :description="$t('skillsSettings.empty')" />
        </div>

        <div v-else class="skills-grid">
          <div
            v-for="skill in displayedSkills"
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
    </template>

    <!-- Skill 内容查看弹窗：左文件树 + 右内容 -->
    <t-dialog
      v-model:visible="detailVisible"
      :header="detailTitle"
      :footer="false"
      width="70%"
      top="8vh"
      dialog-class-name="skill-detail-dialog"
      destroy-on-close
      @close="handleCloseDetail"
    >
      <div class="skill-explorer">
        <!-- 左：文件树 -->
        <div class="skill-tree">
          <div v-if="treeRows.length === 0" class="skill-tree__empty">
            {{ $t('skillsSettings.noFiles') }}
          </div>
          <ul v-else class="tree-list">
            <li
              v-for="row in treeRows"
              :key="row.path"
              class="tree-row"
              :class="{ 'is-dir': row.isDir, 'is-file': !row.isDir, 'is-selected': !row.isDir && row.path === selectedFile }"
              :style="{ paddingLeft: 8 + row.depth * 16 + 'px' }"
              :title="row.path"
              @click="onRowClick(row)"
            >
              <t-icon
                v-if="row.isDir"
                :name="isExpanded(row.path) ? 'folder-open' : 'folder'"
                size="16px"
                class="tree-row__icon"
              />
              <t-icon v-else name="file" size="16px" class="tree-row__icon" />
              <span class="tree-row__name">{{ row.name }}</span>
            </li>
          </ul>
        </div>

        <!-- 右：文件内容 -->
        <div class="skill-content">
          <div v-if="fileLoading" class="skill-content__loading">
            <t-loading :text="$t('common.loading')" />
          </div>

          <div v-else-if="fileError" class="skill-content__error">
            <t-empty :description="$t('skillsSettings.toasts.loadFileFailed')" />
          </div>

          <div v-else-if="!selectedFile" class="skill-content__placeholder">
            <t-empty :description="$t('skillsSettings.selectFileHint')" />
          </div>

          <template v-else>
            <div class="skill-content__path">{{ selectedFile }}</div>
            <div
              v-if="isMarkdown(selectedFile) && fileHtml"
              class="skill-content__md markdown-content"
              v-html="fileHtml"
            ></div>
            <pre v-else class="skill-content__code">{{ fileContent }}</pre>
          </template>
        </div>
      </div>
    </t-dialog>

    <!-- 创建/编辑 Skill 库弹窗 -->
    <t-dialog
      v-model:visible="formDialogVisible"
      :header="formMode === 'edit' ? $t('skillsSettings.editLibrary') : $t('skillsSettings.createLibrary')"
      width="480px"
      :confirm-btn="{ content: formMode === 'edit' ? $t('skillsSettings.save') : $t('skillsSettings.create'), loading: submitting, theme: 'primary' }"
      :cancel-btn="{ content: $t('common.cancel') }"
      destroy-on-close
      @confirm="submitLibraryForm"
      @close="handleCloseFormDialog"
    >
      <div class="create-library-form">
        <div class="create-library-form__row">
          <label class="create-library-form__label">
            <span class="create-library-form__required">*</span>{{ $t('skillsSettings.libraryNameLabel') }}
          </label>
          <t-input
            v-model="libraryForm.name"
            :placeholder="$t('skillsSettings.libraryNamePlaceholder')"
            maxlength="64"
            clearable
            @enter="submitLibraryForm"
          />
        </div>
        <div class="create-library-form__hint">{{ $t('skillsSettings.libraryNameHint') }}</div>
        <div class="create-library-form__row">
          <label class="create-library-form__label">{{ $t('skillsSettings.libraryDescLabel') }}</label>
          <t-textarea
            v-model="libraryForm.description"
            :placeholder="$t('skillsSettings.libraryDescPlaceholder')"
            :autosize="{ minRows: 3, maxRows: 6 }"
            maxlength="500"
            show-count
          />
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { marked } from 'marked'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import { listSkills, getSkill, getSkillFile, createSkillLibrary, updateSkillLibrary, deleteSkillLibrary, type SkillInfo, type SkillLibrary, type SkillDetail } from '@/api/skill'
import { sanitizeMarkdownHTML } from '@/utils/security'
import { configureMarkedForChatMarkdown } from '@/utils/chatMarkdownRenderer'

// embedded = true 时隐藏内部 section-header，由外层页面提供标题栏（用于独立 Skills 页面）
withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

configureMarkedForChatMarkdown()

const { t } = useI18n()
const libraries = ref<SkillLibrary[]>([])
const skillsAvailable = ref(true)
const loading = ref(false)
const currentLibrary = ref<string | null>(null)

const displayedSkills = computed<SkillInfo[]>(() => {
  if (!currentLibrary.value) return []
  const g = libraries.value.find((x) => x.name === currentLibrary.value)
  return g ? (g.skills || []) : []
})

const openLibrary = (name: string) => {
  currentLibrary.value = name
}

const backToLibraries = () => {
  currentLibrary.value = null
}

const loadSkills = async () => {
  loading.value = true
  try {
    const res = await listSkills()
    skillsAvailable.value = res.skills_available !== false
    libraries.value = res.data && res.data.length > 0 ? res.data : []
    // 根目录变更后回到 Skill 库列表层
    currentLibrary.value = null
  } catch (error) {
    skillsAvailable.value = false
    libraries.value = []
    currentLibrary.value = null
    MessagePlugin.error(t('skillsSettings.toasts.loadFailed'))
    console.error('Failed to load skills:', error)
  } finally {
    loading.value = false
  }
}

// ---- 文件树 ----
interface TreeNode {
  name: string
  path: string
  isDir: boolean
  children?: TreeNode[]
}
interface TreeRow {
  name: string
  path: string
  depth: number
  isDir: boolean
}

const skillFiles = ref<string[]>([])
const expandedDirs = ref<Set<string>>(new Set())

function buildTree(files: string[]): TreeNode {
  const root: TreeNode = { name: '', path: '', isDir: true, children: [] }
  for (const f of files) {
    const parts = f.split('/')
    let cur = root
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      const isLast = i === parts.length - 1
      const path = parts.slice(0, i + 1).join('/')
      let child = cur.children!.find((c) => c.name === part)
      if (!child) {
        child = { name: part, path, isDir: !isLast, children: isLast ? undefined : [] }
        cur.children!.push(child)
      }
      cur = child
    }
  }
  return root
}

function collectDirPaths(node: TreeNode, out: string[]) {
  for (const c of node.children || []) {
    if (c.isDir) {
      out.push(c.path)
      collectDirPaths(c, out)
    }
  }
}

const treeRows = computed<TreeRow[]>(() => {
  const rows: TreeRow[] = []
  const walk = (node: TreeNode, depth: number) => {
    const children = [...(node.children || [])].sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
      return a.name.localeCompare(b.name)
    })
    for (const c of children) {
      rows.push({ name: c.name, path: c.path, depth, isDir: c.isDir })
      if (c.isDir && expandedDirs.value.has(c.path)) walk(c, depth + 1)
    }
  }
  walk(buildTree(skillFiles.value), 0)
  return rows
})

const isExpanded = (path: string) => expandedDirs.value.has(path)
const toggleDir = (path: string) => {
  const s = new Set(expandedDirs.value)
  if (s.has(path)) s.delete(path)
  else s.add(path)
  expandedDirs.value = s
}

// ---- 文件内容 ----
const selectedFile = ref('')
const fileContent = ref('')
const fileLoading = ref(false)
const fileError = ref(false)

const isMarkdown = (path: string) => /\.md$/i.test(path)

const fileHtml = computed(() => {
  if (!selectedFile.value || !fileContent.value) return ''
  if (!isMarkdown(selectedFile.value)) return ''
  try {
    return sanitizeMarkdownHTML(String(marked.parse(fileContent.value)))
  } catch (error) {
    console.error('Failed to render skill file markdown:', error)
    return ''
  }
})

const loadFile = async (skillName: string, relPath: string) => {
  selectedFile.value = relPath
  fileContent.value = ''
  fileError.value = false
  fileLoading.value = true
  try {
    const res = await getSkillFile(skillName, relPath, detailLibrary.value)
    fileContent.value = res?.data?.content || ''
  } catch (error) {
    fileError.value = true
    MessagePlugin.error(t('skillsSettings.toasts.loadFileFailed'))
    console.error('Failed to load skill file:', error)
  } finally {
    fileLoading.value = false
  }
}

const onRowClick = (row: TreeRow) => {
  if (row.isDir) {
    toggleDir(row.path)
  } else if (detailName.value) {
    loadFile(detailName.value, row.path)
  }
}

// ---- 弹窗 ----
const detailVisible = ref(false)
const detailName = ref('')
const detailLibrary = ref('')
const detailDescription = ref('')
const detailTitle = computed(() => detailName.value || t('skillsSettings.title'))

const openSkill = async (skill: SkillInfo) => {
  detailName.value = skill.name
  detailLibrary.value = currentLibrary.value || ''
  detailDescription.value = skill.description
  skillFiles.value = []
  selectedFile.value = ''
  fileContent.value = ''
  fileError.value = false
  detailVisible.value = true
  fileLoading.value = true
  try {
    const res = await getSkill(skill.name, detailLibrary.value)
    const detail: SkillDetail | undefined = res?.data
    if (detail) {
      detailDescription.value = detail.description || skill.description
      skillFiles.value = detail.files || []
      // 默认展开所有目录
      const dirs: string[] = []
      collectDirPaths(buildTree(skillFiles.value), dirs)
      expandedDirs.value = new Set(dirs)
      // 默认选中 SKILL.md（若存在），否则第一个文件
      const files = skillFiles.value.slice().sort((a, b) => (a === 'SKILL.md' ? -1 : b === 'SKILL.md' ? 1 : 0))
      const target = files.find((f) => !f.includes('/')) || files[0]
      if (target) {
        loadFile(skill.name, target)
        return
      }
    }
    fileLoading.value = false
  } catch (error) {
    fileError.value = true
    MessagePlugin.error(t('skillsSettings.toasts.loadDetailFailed'))
    console.error('Failed to load skill detail:', error)
    fileLoading.value = false
  }
}

const handleCloseDetail = () => {
  skillFiles.value = []
  expandedDirs.value = new Set()
  selectedFile.value = ''
  fileContent.value = ''
  fileLoading.value = false
  fileError.value = false
}

// ---- 创建/编辑 Skill 库（复用同一表单） ----
const formDialogVisible = ref(false)
const submitting = ref(false)
const formMode = ref<'create' | 'edit'>('create')
const editingOldName = ref('')
const libraryForm = reactive({ name: '', description: '' })

const openCreateDialog = () => {
  formMode.value = 'create'
  editingOldName.value = ''
  libraryForm.name = ''
  libraryForm.description = ''
  formDialogVisible.value = true
}

const openEditDialog = (library: SkillLibrary) => {
  formMode.value = 'edit'
  editingOldName.value = library.name
  libraryForm.name = library.name
  libraryForm.description = library.description || ''
  formDialogVisible.value = true
}

const handleCloseFormDialog = () => {
  if (submitting.value) return
  libraryForm.name = ''
  libraryForm.description = ''
}

const submitLibraryForm = async () => {
  const name = libraryForm.name.trim()
  if (!name) {
    MessagePlugin.warning(t('skillsSettings.libraryNameRequired'))
    return
  }
  const description = libraryForm.description.trim()
  submitting.value = true
  try {
    if (formMode.value === 'edit') {
      await updateSkillLibrary(editingOldName.value, name, description)
      MessagePlugin.success(t('skillsSettings.toasts.updateLibraryOk'))
      // 库名可能已变，回到库列表层
      currentLibrary.value = null
    } else {
      await createSkillLibrary(name, description)
      MessagePlugin.success(t('skillsSettings.toasts.createLibraryOk'))
    }
    formDialogVisible.value = false
    await loadSkills()
  } catch (error: any) {
    const fallback = formMode.value === 'edit'
      ? t('skillsSettings.toasts.updateLibraryFailed')
      : t('skillsSettings.toasts.createLibraryFailed')
    const msg = error?.response?.data?.message || error?.message || fallback
    MessagePlugin.error(msg)
    console.error('Failed to submit skill library form:', error)
  } finally {
    submitting.value = false
  }
}

// ---- 库卡片操作下拉 ----
const libraryActionOptions = computed(() => [
  { content: t('skillsSettings.editLibrary'), value: 'edit' },
  { content: t('skillsSettings.deleteLibrary'), value: 'delete', theme: 'error' as const },
])

const onLibraryAction = (opt: { value: string }, library: SkillLibrary) => {
  if (opt?.value === 'edit') {
    openEditDialog(library)
  } else if (opt?.value === 'delete') {
    confirmDeleteLibrary(library)
  }
}

// ---- 删除 Skill 库 ----
const confirmDeleteLibrary = (library: SkillLibrary) => {
  const confirmDialog = DialogPlugin.confirm({
    header: t('skillsSettings.deleteLibrary'),
    body: t('skillsSettings.deleteConfirmBody', { name: library.name, count: (library.skills || []).length }),
    theme: 'warning',
    confirmBtn: { content: t('skillsSettings.delete'), theme: 'danger', loading: false },
    cancelBtn: { content: t('common.cancel') },
    onConfirm: async () => {
      confirmDialog.update({ confirmBtn: { content: t('skillsSettings.delete'), theme: 'danger', loading: true } })
      try {
        await deleteSkillLibrary(library.name)
        MessagePlugin.success(t('skillsSettings.toasts.deleteLibraryOk'))
        confirmDialog.destroy()
        // 若删的正是当前进入的库，退回列表层
        if (currentLibrary.value === library.name) currentLibrary.value = null
        await loadSkills()
      } catch (error: any) {
        const msg = error?.response?.data?.message || error?.message || t('skillsSettings.toasts.deleteLibraryFailed')
        MessagePlugin.error(msg)
        console.error('Failed to delete skill library:', error)
        confirmDialog.update({ confirmBtn: { content: t('skillsSettings.delete'), theme: 'danger', loading: false } })
      }
    },
  })
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

.list-section-header--with-back {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;

  .list-section-header__text {
    min-width: 0;
  }

  .list-section-header__actions {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
  }

  .back-to-libraries-btn {
    flex-shrink: 0;
    white-space: nowrap;
  }
}

.libraries-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
}

.library-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 14px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.15s ease, box-shadow 0.15s ease, background 0.15s ease;

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 2px 12px rgba(0, 82, 217, 0.12);

    .library-card__actions {
      opacity: 1;
    }
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

.library-card__badge {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 82, 217, 0.1);
  color: #0052D9;
}

.library-card__body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.library-card__title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.4;
  color: var(--td-text-color-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.library-card__desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-secondary);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.library-card__count {
  font-size: 12px;
  color: var(--td-text-color-placeholder);
}

.library-card__arrow {
  flex-shrink: 0;
  color: var(--td-text-color-placeholder);
}

.library-card__actions {
  position: absolute;
  top: 6px;
  right: 6px;
  opacity: 0;
  transition: opacity 0.15s ease;
  z-index: 1;

  // 始终允许键盘聚焦时可见
  &:focus-within {
    opacity: 1;
  }
}

.library-card__actions-btn {
  // 让「操作」按钮在卡片右上角低调可点
  --td-comp-padding-vertical-s: 0px;
  padding: 2px 6px;
  font-size: 12px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-secondarycontainer);
  border-radius: 6px;

  &:hover {
    color: var(--td-brand-color);
    background: var(--td-bg-color-secondarycontainer-hover, var(--td-bg-color-secondarycontainer));
  }
}

.library-card--create {
  border-style: dashed;
  border-color: var(--td-component-border);
  background: transparent;
  align-items: center;

  &:hover {
    border-color: var(--td-brand-color);
    background: var(--td-bg-color-secondarycontainer);
    box-shadow: none;
  }
}

.library-card__badge--create {
  background: rgba(0, 82, 217, 0.08);
  color: var(--td-brand-color);
}

// ---- 创建库表单 ----
.create-library-form {
  display: flex;
  flex-direction: column;
  gap: 4px;

  &__row {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 8px;
  }

  &__label {
    font-size: 13px;
    color: var(--td-text-color-primary);
    font-weight: 500;
  }

  &__required {
    color: var(--td-error-color, #e34d59);
    margin-right: 4px;
  }

  &__hint {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    line-height: 1.5;
    margin-bottom: 4px;
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

// ---- 弹窗内：左树右内容 ----
.skill-explorer {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  gap: 1px;
  background: var(--td-component-stroke);
  border-radius: 8px;
  overflow: hidden;
}

.skill-tree {
  flex: 0 0 240px;
  width: 240px;
  background: var(--td-bg-color-container);
  overflow-y: auto;
  padding: 8px 0;

  &__empty {
    padding: 24px 12px;
    font-size: 13px;
    color: var(--td-text-color-placeholder);
    text-align: center;
  }
}

.tree-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.tree-row {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px 5px 8px;
  font-size: 13px;
  line-height: 1.4;
  color: var(--td-text-color-primary);
  cursor: pointer;
  user-select: none;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;

  &__icon {
    flex-shrink: 0;
    color: var(--td-text-color-secondary);
  }

  &__name {
    overflow: hidden;
    text-overflow: ellipsis;
  }

  &.is-dir {
    font-weight: 500;

    &:hover {
      background: var(--td-bg-color-secondarycontainer);
    }
  }

  &.is-file {
    &:hover {
      background: var(--td-bg-color-secondarycontainer);
    }

    &.is-selected {
      background: rgba(0, 82, 217, 0.1);
      color: var(--td-brand-color);
    }
  }
}

.skill-content {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  background: var(--td-bg-color-container);
  overflow-y: auto;
  padding: 12px 16px;

  &__loading,
  &__error,
  &__placeholder {
    padding: 48px 0;
    text-align: center;
  }

  &__path {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    font-family: var(--td-font-family-mono, monospace);
    padding: 4px 8px;
    margin-bottom: 8px;
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 6px;
    word-break: break-all;
  }

  &__md {
    .chat-markdown-typography();
  }

  &__code {
    margin: 0;
    padding: 12px;
    font-family: var(--td-font-family-mono, monospace);
    font-size: 12.5px;
    line-height: 1.6;
    color: var(--td-text-color-primary);
    background: var(--td-bg-color-secondarycontainer);
    border-radius: 6px;
    white-space: pre-wrap;
    word-break: break-word;
  }
}
</style>

<style lang="less">
// Dialog 被 teleport 到 body，scoped 样式够不着，需用非 scoped 样式约束。
// 注意：dialogClassName 是加在 .t-dialog 元素自身上（与 t-dialog 同一元素），
// 所以直接选中 .skill-detail-dialog，而不是 .skill-detail-dialog .t-dialog。
.skill-detail-dialog {
  max-height: 88vh;
  // 浏览器原生拖拽调整大小：右下角出现拖拽手柄，按住即可改变宽高，
  // 内部 flex 布局自适应。resize 要求 overflow 非 visible。
  resize: both;
  overflow: hidden;
  min-width: 480px;
  min-height: 320px;
  max-width: 96vw;
  display: flex;
  flex-direction: column;

  .t-dialog__body {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
}
</style>
