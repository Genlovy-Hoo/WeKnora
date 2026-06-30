<!-- AI-generated: 工具管理页面（一等公民）。两级导航：工具库列表 → 库内工具列表。
     支持库 CRUD、工具 CRUD、启停、编辑、测试。内置库只读。 -->
<template>
  <div class="tools-page">
    <header class="tools-page__header">
      <div class="tools-page__titlewrap">
        <t-icon name="tools" class="tools-page__icon" />
        <div>
          <h1 class="tools-page__title">{{ $t('menu.tools') }}</h1>
          <p class="tools-page__desc">{{ $t('tools.pageDesc') }}</p>
        </div>
      </div>
    </header>

    <div class="tools-page__body">
      <t-loading :loading="loading" size="small">
        <!-- 第一层：工具库列表 -->
        <template v-if="!currentLibrary">
          <div class="list-section-header">
            <h3>{{ $t('tools.libraries') }}</h3>
            <p>{{ $t('tools.librariesHint') }}</p>
          </div>

          <div class="libraries-grid">
            <div
              v-for="lib in libraries"
              :key="lib.id"
              class="library-card"
              role="button"
              tabindex="0"
              @click="openLibrary(lib)"
              @keydown.enter.prevent="openLibrary(lib)"
              @keydown.space.prevent="openLibrary(lib)"
            >
              <div class="library-card__badge">
                <t-icon :name="lib.is_builtin ? 'star' : 'folder'" size="20px" />
              </div>
              <div class="library-card__body">
                <h3 class="library-card__title">
                  {{ lib.name }}
                  <t-tag v-if="lib.is_builtin" theme="primary" size="small" variant="light">{{ $t('tools.builtin') }}</t-tag>
                </h3>
                <div class="library-card__desc">{{ lib.description || $t('tools.noDesc') }}</div>
                <div class="library-card__count">{{ lib.tools.length }} {{ $t('tools.toolsUnit') }}</div>
              </div>
              <t-icon name="chevron-right" size="18px" class="library-card__arrow" />

              <div v-if="!lib.is_builtin" class="library-card__actions" @click.stop @keydown.stop>
                <t-dropdown :options="libActionOptions" trigger="click" @click="(opt: any) => onLibAction(opt, lib)">
                  <t-button theme="default" variant="text" size="small">
                    {{ $t('tools.actions') }}
                    <template #suffix><t-icon name="chevron-down" size="14px" /></template>
                  </t-button>
                </t-dropdown>
              </div>
            </div>

            <div
              class="library-card library-card--create"
              role="button"
              tabindex="0"
              @click="openCreateLibDialog"
              @keydown.enter.prevent="openCreateLibDialog"
              @keydown.space.prevent="openCreateLibDialog"
            >
              <div class="library-card__badge library-card__badge--create"><t-icon name="add" size="20px" /></div>
              <div class="library-card__body">
                <h3 class="library-card__title">{{ $t('tools.createLibrary') }}</h3>
                <div class="library-card__desc">{{ $t('tools.createLibraryHint') }}</div>
              </div>
            </div>
          </div>

          <div v-if="libraries.length === 0" class="empty-state">
            <t-empty :description="$t('tools.emptyLibraries')" />
          </div>
        </template>

        <!-- 第二层：库内工具列表 -->
        <template v-else>
          <div class="list-section-header list-section-header--with-back">
            <div class="list-section-header__text">
              <h3>{{ currentLibrary.name }}</h3>
              <p>{{ $t('tools.manageHint') }}</p>
            </div>
            <div class="list-section-header__actions">
              <t-button v-if="!currentLibrary.is_builtin" theme="primary" @click="openCreateToolDialog">
                <template #icon><t-icon name="add" /></template>
                {{ $t('tools.createTool') }}
              </t-button>
              <t-button theme="default" variant="text" @click="backToLibraries">
                <template #icon><t-icon name="arrow-left" /></template>
                {{ $t('tools.backToLibraries') }}
              </t-button>
            </div>
          </div>

          <div v-if="currentLibrary.tools.length === 0" class="empty-state">
            <t-empty :description="$t('tools.emptyTools')" />
          </div>

          <div v-else class="tools-grid">
            <div v-for="tool in currentLibrary.tools" :key="tool.name" class="tool-card">
              <div class="tool-card__head">
                <div class="tool-card__name">
                  <t-icon name="link" size="16px" />
                  <span :title="tool.name">{{ tool.display_name || tool.name }}</span>
                </div>
                <t-tag size="small" variant="outline">{{ tool.group }}</t-tag>
              </div>
              <div class="tool-card__desc" :title="tool.description">{{ tool.description }}</div>
              <div class="tool-card__footer">
                <div class="tool-card__meta">
                  <t-tag v-if="tool.danger" theme="warning" size="small" variant="light">{{ $t('tools.requireApproval') }}</t-tag>
                  <span v-if="!currentLibrary.is_builtin" class="tool-card__statename" :class="{ 'is-off': !getToolEnabled(tool) }">
                    {{ getToolEnabled(tool) ? $t('tools.enabled') : $t('tools.disabled') }}
                  </span>
                </div>
                <div v-if="!currentLibrary.is_builtin" class="tool-card__ops">
                  <t-button theme="default" variant="text" size="small" @click="onToggleTool(tool)">
                    {{ getToolEnabled(tool) ? $t('tools.disable') : $t('tools.enable') }}
                  </t-button>
                  <t-button theme="default" variant="text" size="small" @click="onEditTool(tool)">{{ $t('tools.edit') }}</t-button>
                  <t-popconfirm :content="$t('tools.deleteConfirm')" @confirm="onDeleteTool(tool)">
                    <t-button theme="danger" variant="text" size="small">{{ $t('tools.delete') }}</t-button>
                  </t-popconfirm>
                </div>
              </div>
            </div>
          </div>
        </template>
      </t-loading>
    </div>

    <!-- 库新建/编辑弹窗 -->
    <t-dialog
      v-model:visible="libDialogVisible"
      :header="libDialogEdit ? $t('tools.editLibrary') : $t('tools.createLibrary')"
      width="420px"
      @confirm="onSubmitLib"
    >
      <t-form :data="libForm" label-align="top">
        <t-form-item name="name">
          <template #label>
            <span class="required-star">*</span><span>{{ $t('tools.libraryName') }}</span>
          </template>
          <t-input v-model="libForm.name" :placeholder="$t('tools.libraryNamePlaceholder')" />
        </t-form-item>
        <t-form-item :label="$t('tools.libraryDesc')" name="description">
          <t-textarea v-model="libForm.description" :autosize="{ minRows: 2, maxRows: 4 }" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 工具编辑弹窗 -->
    <ToolEditorModal
      v-model:visible="toolDialogVisible"
      :library-id="currentLibrary?.id || ''"
      :edit-tool="editingTool"
      @saved="onToolSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import {
  listAllTools,
  listCustomToolsByLibrary,
  createToolLibrary,
  updateToolLibrary,
  deleteToolLibrary,
  updateCustomTool,
  deleteCustomTool,
  type ToolLibraryWithTools,
  type CustomTool,
  type CustomToolRequest,
} from '@/api/tool'
import ToolEditorModal from './ToolEditorModal.vue'

const loading = ref(false)
const libraries = ref<ToolLibraryWithTools[]>([])
const currentLibrary = ref<ToolLibraryWithTools | null>(null)
// 工具列表缓存：库ID -> 自定义工具详情（含 enabled）。聚合 API 不返回 enabled，按库单独拉。
const customToolsByLib = ref<Record<string, CustomTool[]>>({})

const libDialogVisible = ref(false)
const libDialogEdit = ref(false)
const libForm = ref<{ name: string; description: string }>({ name: '', description: '' })
let editingLibId = ''

const toolDialogVisible = ref(false)
const editingTool = ref<CustomTool | null>(null)

const libActionOptions = computed(() => [
  { content: '编辑', value: 'edit' },
  { content: '删除', value: 'delete', theme: 'error' },
])

async function load() {
  loading.value = true
  try {
    const res: any = await listAllTools()
    libraries.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function loadCustomTools(libId: string) {
  const res: any = await listCustomToolsByLibrary(libId)
  customToolsByLib.value[libId] = res.data || []
}

function getToolEnabled(tool: { name: string }): boolean {
  if (!currentLibrary.value || currentLibrary.value.is_builtin) return true
  const list = customToolsByLib.value[currentLibrary.value.id] || []
  const found = list.find((t) => t.name === tool.name)
  return found ? found.enabled : true
}

function openLibrary(lib: ToolLibraryWithTools) {
  currentLibrary.value = lib
  if (!lib.is_builtin) {
    loadCustomTools(lib.id).catch(() => {})
  }
}

function backToLibraries() {
  currentLibrary.value = null
}

function openCreateLibDialog() {
  libDialogEdit.value = false
  libForm.value = { name: '', description: '' }
  editingLibId = ''
  libDialogVisible.value = true
}

function onLibAction(opt: { value: string }, lib: ToolLibraryWithTools) {
  if (opt.value === 'edit') {
    libDialogEdit.value = true
    editingLibId = lib.id
    libForm.value = { name: lib.name, description: lib.description || '' }
    libDialogVisible.value = true
  } else if (opt.value === 'delete') {
    const dlg = DialogPlugin.confirm({
      header: '删除工具库',
      body: `确定删除工具库「${lib.name}」？其下所有自定义工具将一并删除。`,
      onConfirm: async () => {
        try {
          await deleteToolLibrary(lib.id)
          MessagePlugin.success('已删除')
          await load()
          dlg.destroy()
        } catch (e: any) {
          MessagePlugin.error(e?.message || '删除失败')
        }
      },
    })
  }
}

async function onSubmitLib() {
  if (!libForm.value.name.trim()) {
    MessagePlugin.warning('工具库名称不能为空')
    return
  }
  try {
    if (libDialogEdit.value) {
      await updateToolLibrary(editingLibId, libForm.value.name, libForm.value.description)
    } else {
      await createToolLibrary(libForm.value.name, libForm.value.description)
    }
    MessagePlugin.success('保存成功')
    libDialogVisible.value = false
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

function openCreateToolDialog() {
  editingTool.value = null
  toolDialogVisible.value = true
}

function onEditTool(tool: { name: string }) {
  if (!currentLibrary.value) return
  const list = customToolsByLib.value[currentLibrary.value.id] || []
  editingTool.value = list.find((t) => t.name === tool.name) || null
  toolDialogVisible.value = true
}

async function onToolSaved() {
  if (currentLibrary.value && !currentLibrary.value.is_builtin) {
    await loadCustomTools(currentLibrary.value.id)
  }
  await load()
}

async function onToggleTool(tool: { name: string }) {
  if (!currentLibrary.value) return
  const list = customToolsByLib.value[currentLibrary.value.id] || []
  const t = list.find((x) => x.name === tool.name)
  if (!t) return
  const req: CustomToolRequest = {
    name: t.name,
    display_name: t.display_name,
    description: t.description,
    parameters_schema: t.parameters_schema,
    http_config: {
      method: t.http_config.method,
      url_template: t.http_config.url_template,
      headers: t.http_config.headers,
      body_template: t.http_config.body_template,
      response_extract: t.http_config.response_extract,
      timeout_sec: t.http_config.timeout_sec,
    },
    enabled: !t.enabled,
    require_approval: t.require_approval,
  }
  try {
    await updateCustomTool(t.id, req)
    MessagePlugin.success(!t.enabled ? '已启用' : '已停用')
    await loadCustomTools(currentLibrary.value.id)
  } catch (e: any) {
    MessagePlugin.error(e?.message || '操作失败')
  }
}

async function onDeleteTool(tool: { name: string }) {
  if (!currentLibrary.value) return
  const list = customToolsByLib.value[currentLibrary.value.id] || []
  const t = list.find((x) => x.name === tool.name)
  if (!t) return
  try {
    await deleteCustomTool(t.id)
    MessagePlugin.success('已删除')
    await loadCustomTools(currentLibrary.value.id)
    await load()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}

onMounted(load)
</script>

<style lang="less" scoped>
.tools-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}
.tools-page__header {
  flex-shrink: 0;
  padding: 20px 28px 14px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}
.tools-page__titlewrap {
  display: flex;
  align-items: center;
  gap: 12px;
}
.tools-page__icon {
  font-size: 26px;
  color: var(--td-brand-color);
  flex-shrink: 0;
}
.tools-page__title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  color: var(--td-text-color-primary);
}
.tools-page__desc {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--td-text-color-secondary);
}
.tools-page__body {
  flex: 1;
  overflow: auto;
  padding: 20px 28px;
}
.list-section-header {
  margin-bottom: 16px;
  h3 { margin: 0; font-size: 16px; font-weight: 600; }
  p { margin: 4px 0 0; font-size: 12px; color: var(--td-text-color-secondary); }
  &--with-back {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
  }
  &__actions {
    display: flex;
    gap: 8px;
    align-items: center;
  }
}
.libraries-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
}
.library-card {
  position: relative;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border-radius: 8px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  cursor: pointer;
  transition: border-color 0.2s, box-shadow 0.2s;
  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
    .library-card__actions { opacity: 1; }
  }
  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
    .library-card__actions { opacity: 1; }
  }
  &--create {
    border-style: dashed;
    color: var(--td-brand-color);
  }
  &__badge {
    width: 40px; height: 40px;
    border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    background: var(--td-brand-color-1);
    color: var(--td-brand-color);
    flex-shrink: 0;
    &--create { background: var(--td-brand-color-light); }
  }
  &__body { flex: 1; min-width: 0; }
  &__title {
    margin: 0; font-size: 14px; font-weight: 600;
    display: flex; align-items: center; gap: 6px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  &__desc {
    margin-top: 4px; font-size: 12px; color: var(--td-text-color-secondary);
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  &__count {
    margin-top: 6px; font-size: 12px; color: var(--td-text-color-secondary);
  }
  &__arrow { color: var(--td-text-color-placeholder); flex-shrink: 0; }
  &__actions {
    position: absolute; top: 8px; right: 8px;
    opacity: 0;
    transition: opacity 0.15s ease;
    z-index: 1;
  }
}
.tools-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 12px;
}
.tool-card {
  padding: 14px;
  border-radius: 8px;
  border: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
  display: flex;
  flex-direction: column;
  gap: 8px;
  &__head {
    display: flex; align-items: center; justify-content: space-between; gap: 8px;
  }
  &__name {
    display: flex; align-items: center; gap: 6px;
    font-weight: 600; font-size: 14px;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  &__desc {
    font-size: 12px; color: var(--td-text-color-secondary);
    display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical;
    overflow: hidden;
    min-height: 32px;
  }
  &__footer {
    display: flex; align-items: center; justify-content: space-between;
  }
  &__meta { display: flex; align-items: center; gap: 8px; }
  &__statename {
    font-size: 12px; color: var(--td-success-color-5);
    &.is-off { color: var(--td-text-color-placeholder); }
  }
  &__ops { display: flex; gap: 4px; }
}
.empty-state { padding: 40px 0; }
.required-star {
  color: var(--td-error-color, #e34d59);
  margin-right: 4px;
}
</style>
