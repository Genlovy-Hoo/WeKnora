import { get, post, put, del } from "../../utils/request";

// Skill信息
export interface SkillInfo {
  name: string;
  description: string;
}

// Skill 库（library）：skills 根目录下的一个一级子目录，包含若干 skill
export interface SkillLibrary {
  name: string;
  description?: string;
  skills: SkillInfo[];
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

// 获取Skills列表（按库分组）；skills_available 为 false 表示沙箱未启用，前端应隐藏/禁用 Skills 配置
export function listSkills() {
  return get<{ data: SkillLibrary[]; skills_available?: boolean }>('/api/v1/skills');
}

// 创建 Skill 库（在 skills root 下新建一个以 name 命名的文件夹）
export function createSkillLibrary(name: string, description: string) {
  return post<{ data: null; message?: string }>('/api/v1/skills/libraries', { name, description });
}

// 更新 Skill 库（可改名文件夹 + 更新描述）
export function updateSkillLibrary(oldName: string, name: string, description: string) {
  return put<{ data: null; message?: string }>(`/api/v1/skills/libraries/${encodeURIComponent(oldName)}`, { name, description });
}

// 删除 Skill 库（连同其下所有 skills）
export function deleteSkillLibrary(name: string) {
  return del<{ data: null; message?: string }>(`/api/v1/skills/libraries/${encodeURIComponent(name)}`);
}

// 获取单个 Skill 的内容（元数据 + instructions 正文 + 文件树）
// library 为该 skill 所在的 Skill 库名，跨库重名时用于按库精确解析。
export function getSkill(name: string, library: string) {
  return get<{ data: SkillDetail }>(
    `/api/v1/skills/${encodeURIComponent(name)}?library=${encodeURIComponent(library)}`,
  );
}

// 获取 Skill 内单个文件的内容
// library 为该 skill 所在的 Skill 库名，跨库重名时用于按库精确解析。
export function getSkillFile(name: string, relPath: string, library: string) {
  return get<{ data: SkillFileContent }>(
    `/api/v1/skills/${encodeURIComponent(name)}/file?path=${encodeURIComponent(relPath)}&library=${encodeURIComponent(library)}`,
  );
}
