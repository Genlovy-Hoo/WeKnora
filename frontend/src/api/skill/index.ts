import { get } from "../../utils/request";

// Skill信息
export interface SkillInfo {
  name: string;
  description: string;
}

// Skill 详情（含 SKILL.md 正文 + 文件树）
export interface SkillDetail {
  name: string;
  description: string;
  instructions: string;
  files: string[];
}

// Skill 内单个文件内容
export interface SkillFileContent {
  name: string;      // 相对路径
  content: string;
  is_script: boolean;
}

// 获取Skills列表；skills_available 为 false 表示沙箱未启用，前端应隐藏/禁用 Skills 配置
export function listSkills() {
  return get<{ data: SkillInfo[]; skills_available?: boolean }>('/api/v1/skills');
}

// 获取单个 Skill 的内容（元数据 + instructions 正文 + 文件树）
export function getSkill(name: string) {
  return get<{ data: SkillDetail }>(`/api/v1/skills/${encodeURIComponent(name)}`);
}

// 获取 Skill 内单个文件的内容
export function getSkillFile(name: string, relPath: string) {
  return get<{ data: SkillFileContent }>(
    `/api/v1/skills/${encodeURIComponent(name)}/file?path=${encodeURIComponent(relPath)}`,
  );
}
