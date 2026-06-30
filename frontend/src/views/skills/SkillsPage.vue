<!-- AI-generated: Skills 独立管理页面，复用 SkillSettings 组件，外层提供页面标题栏。 -->
<template>
  <div class="skills-page">
    <header class="skills-page__header">
      <div class="skills-page__titlewrap">
        <t-icon name="lightbulb" class="skills-page__icon" />
        <div>
          <h1 class="skills-page__title">{{ $t('skillsSettings.title') }}</h1>
          <p class="skills-page__desc">{{ $t('skillsSettings.description') }}</p>
        </div>
      </div>
    </header>
    <div class="skills-page__body">
      <SkillSettings embedded />
    </div>
  </div>
</template>

<script setup lang="ts">
import SkillSettings from '@/views/settings/SkillSettings.vue'
</script>

<style lang="less" scoped>
.skills-page {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.skills-page__header {
  flex-shrink: 0;
  padding: 20px 28px 14px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--td-bg-color-container);
}

.skills-page__titlewrap {
  display: flex;
  align-items: center;
  gap: 12px;
}

.skills-page__icon {
  font-size: 26px;
  color: var(--td-brand-color);
  flex-shrink: 0;
}

.skills-page__title {
  margin: 0;
  font-size: 18px;
  font-weight: 600;
  line-height: 24px;
  color: var(--td-text-color-primary);
}

.skills-page__desc {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--td-text-color-secondary);
  line-height: 18px;
}

.skills-page__body {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 20px 28px 28px;
}
</style>
